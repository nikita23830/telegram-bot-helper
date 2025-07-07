package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log"
	_ "modernc.org/sqlite"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

type Ticket struct {
	UserID     string
	UserChatID string
	ThreadID   string
	LastUpdate time.Time
}

var db *sql.DB

type Config struct {
	Token        string `json:"token"`
	ChatID       int64  `json:"chat_id"`
	FirstMessage string `json:"first_message"`
	ExpiryDay    int    `json:"expiry_day"`
}

var defaultConfig = Config{
	Token:        "TOKEN_TG_BOT",
	ChatID:       1,
	FirstMessage: "Приветствие",
	ExpiryDay:    15,
}

func LoadConfig(filename string) Config {
	file, err := os.Open(filename)
	if err != nil {
		log.Printf("Не удалось открыть файл %s, используем дефолтные значения: %v", filename, err)
		saveConfig(filename, defaultConfig)
		return defaultConfig
	}
	defer file.Close()

	var cfg Config = defaultConfig // стартуем с дефолтами

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		log.Printf("Ошибка при парсинге %s, используем дефолт: %v", filename, err)
		saveConfig(filename, defaultConfig)
		return defaultConfig
	}

	return cfg
}

func saveConfig(filename string, cfg Config) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Printf("Не удалось сериализовать конфиг: %v", err)
		return
	}

	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		log.Printf("Не удалось сохранить конфиг в %s: %v", filename, err)
	}
}

func main() {
	defaultConfig = LoadConfig("./config.json")
	var err error
	db, err = sql.Open("sqlite", "./tickets.db")
	if err != nil {
		log.Fatal("Open error:", err)
	}
	defer db.Close()

	// Попытка создать таблицу
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tickets (
			user_id TEXT PRIMARY KEY,
			user_chat_id TEXT NOT NULL,
			thread_id TEXT NOT NULL,
			last_update DATETIME NOT NULL
		)
	`)
	if err != nil {
		log.Fatal("Exec error:", err)
	}

	fmt.Println("БД и таблица готовы")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	opts := []bot.Option{
		bot.WithDefaultHandler(handler),
	}

	b, err := bot.New(defaultConfig.Token, opts...)
	if err != nil {
		panic(err)
	}

	go checker()

	b.Start(ctx)
}

func FilterExpiredTickets(tickets []Ticket, expiryDays int) []Ticket {
	var expired []Ticket
	cutoff := time.Now().Add(-time.Duration(expiryDays) * 24 * time.Hour)

	for _, t := range tickets {
		if t.LastUpdate.Before(cutoff) {
			expired = append(expired, t)
		}
	}

	return expired
}

func remove() {
	tk, _ := LoadAllTickets(db)
	tics := FilterExpiredTickets(tk, defaultConfig.ExpiryDay)
	for _, ti := range tics {
		db.Exec(`DELETE FROM tickets WHERE user_id = ?`, ti.UserID)
	}
}

func checker() {
	ticker := time.NewTicker(6 * time.Hour)
	quit := make(chan struct{})

	go func() {
		time.Sleep(1 * time.Minute)
		remove()

		for {
			select {
			case <-ticker.C:
				remove()
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
}

func LoadAllTickets(db *sql.DB) ([]Ticket, error) {
	rows, err := db.Query(`SELECT user_id, user_chat_id, thread_id, last_update FROM tickets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Ticket
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(&t.UserID, &t.UserChatID, &t.ThreadID, &t.LastUpdate); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func GetTicketByUserID(db *sql.DB, userID string) (*Ticket, error) {
	var ticket Ticket
	err := db.QueryRow(`
        SELECT user_id, user_chat_id, thread_id, last_update
        FROM tickets
        WHERE user_id = ?
    `, userID).Scan(&ticket.UserID, &ticket.UserChatID, &ticket.ThreadID, &ticket.LastUpdate)
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

func GetTicketByThreadID(db *sql.DB, threadID string) (*Ticket, error) {
	var ticket Ticket
	err := db.QueryRow(`
        SELECT user_id, user_chat_id, thread_id, last_update
        FROM tickets
        WHERE thread_id = ?
    `, threadID).Scan(&ticket.UserID, &ticket.UserChatID, &ticket.ThreadID, &ticket.LastUpdate)
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

func SaveTicket(db *sql.DB, ticket *Ticket) error {
	_, err := db.Exec(`
        INSERT INTO tickets (user_id, user_chat_id, thread_id, last_update)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			user_chat_id = excluded.user_chat_id,
			thread_id = excluded.thread_id,
			last_update = excluded.last_update;
    `, ticket.UserID, ticket.UserChatID, ticket.ThreadID, time.Now())
	return err
}

func handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	if update.Message.From.IsBot {
		return
	}
	if update.Message.Text == "/start" {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Укажите свой ник и напишите, с чем вам помочь.",
		})
		return
	}
	if update.Message.Chat.ID == defaultConfig.ChatID {
		thread, e := GetTicketByThreadID(db, strconv.Itoa(update.Message.MessageThreadID))
		if e != nil || thread == nil {
			return
		}
		a, _ := strconv.Atoi(thread.UserChatID)
		var centity []models.MessageEntity
		for _, k := range update.Message.CaptionEntities {
			centity = append(centity, models.MessageEntity{
				Type:          k.Type,
				Offset:        k.Offset,
				Length:        k.Length,
				URL:           k.URL,
				User:          nil,
				Language:      k.Language,
				CustomEmojiID: k.CustomEmojiID,
			})
		}
		switch forwardMedia(update) {
		case 0:
			{
				fileID := update.Message.Photo[len(update.Message.Photo)-1].FileID
				b.SendPhoto(ctx, &bot.SendPhotoParams{
					ChatID: int64(a),
					Photo: &models.InputFileString{
						Data: fileID,
					},
					Caption:         update.Message.Caption,
					CaptionEntities: centity,
				})
				break
			}
		case 1:
			{
				fileID := update.Message.Document.FileID
				b.SendDocument(ctx, &bot.SendDocumentParams{
					ChatID: int64(a),
					Document: &models.InputFileString{
						Data: fileID,
					},
					Caption:         update.Message.Caption,
					CaptionEntities: centity,
				})
				break
			}
		case 3:
			{
				b.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: int64(a),
					Text:   update.Message.Text,
				})
				break
			}
		}
		SaveTicket(db, thread)
		return
	}
	var top int64
	var tick *Ticket
	var find bool
	var err error
	tick, err = GetTicketByUserID(db, strconv.Itoa(int(update.Message.From.ID)))
	if err != nil || tick == nil || tick.ThreadID == "0" {
		title := update.Message.From.Username
		if len(strings.Replace(title, " ", "", 9)) == 0 {
			title = "TG User: " + strconv.Itoa(int(update.Message.From.ID))
		}
		t, e := b.CreateForumTopic(ctx, &bot.CreateForumTopicParams{
			ChatID: defaultConfig.ChatID,
			Name:   title,
		})
		if e != nil {
			println(e.Error())
		}
		tick = &Ticket{
			ThreadID:   strconv.Itoa(t.MessageThreadID),
			UserID:     strconv.Itoa(int(update.Message.From.ID)),
			UserChatID: strconv.Itoa(int(update.Message.Chat.ID)),
		}
		SaveTicket(db, tick)
		println("Топик для " + update.Message.From.Username + " [" + strconv.Itoa(int(update.Message.From.ID)) + "] не найден. Создается... " + strconv.Itoa(t.MessageThreadID))
		find = false
	} else {
		find = true
	}
	a, _ := strconv.Atoi(tick.ThreadID)
	top = int64(a)

	var centity []models.MessageEntity
	for _, k := range update.Message.CaptionEntities {
		centity = append(centity, models.MessageEntity{
			Type:          k.Type,
			Offset:        k.Offset,
			Length:        k.Length,
			URL:           k.URL,
			User:          nil,
			Language:      k.Language,
			CustomEmojiID: k.CustomEmojiID,
		})
	}
	switch forwardMedia(update) {
	case 0:
		{
			fileID := update.Message.Photo[len(update.Message.Photo)-1].FileID
			b.SendPhoto(ctx, &bot.SendPhotoParams{
				ChatID: defaultConfig.ChatID,
				Photo: &models.InputFileString{
					Data: fileID,
				},
				MessageThreadID: int(top),
				Caption:         update.Message.Caption,
				CaptionEntities: centity,
			})
			break
		}
	case 1:
		{
			fileID := update.Message.Document.FileID
			b.SendDocument(ctx, &bot.SendDocumentParams{
				ChatID: defaultConfig.ChatID,
				Document: &models.InputFileString{
					Data: fileID,
				},
				MessageThreadID: int(top),
				Caption:         update.Message.Caption,
				CaptionEntities: centity,
			})
			break
		}
	case 3:
		{
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          defaultConfig.ChatID,
				Text:            update.Message.Text,
				MessageThreadID: int(top),
			})
			break
		}
	}
	SaveTicket(db, tick)
	if !find {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   defaultConfig.FirstMessage,
		})
	}
}

func forwardMedia(update *models.Update) int {
	msg := update.Message
	switch {
	case msg.Photo != nil && len(msg.Photo) > 0:
		return 0
	case msg.Document != nil:
		return 1
	// и т.д.
	default:
		return 3
	}
}
