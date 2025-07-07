# Telegram Bot Helper

[![Release](https://img.shields.io/github/v/release/nikita23830/telegram-bot-helper)](https://github.com/nikita23830/telegram-bot-helper/releases)
[![License](https://img.shields.io/github/license/nikita23830/telegram-bot-helper)](LICENSE)

A simple Telegram bot assistant with support for basic functions for automating tasks.

## 📦 Description

This bot is designed to help automate routine actions via Telegram. Suitable for personal use or small teams. Supports convenient connection and configuration via configuration files.

**Main features:**
- Quick setup via the config file
- Easy deployment on any VPS/server
- Flexibility and extensibility for your tasks

## 🚀 Installation

1. Download the compiled binary script from [release](https://github.com/nikita23830/telegram-bot-helper/releases ).
2. Prepare the configuration file (an example is given in the release description).
3. Launch the bot with the command:
   ```bash
   ./tg_bot

##⚙️ Configuration
All settings are specified in the configuration file, which must be placed in the same directory as the binary file of the bot. In the configuration example:

TG_BOT_TOKEN is a Telegram bot token received through @BotFather.
TG_CHAT_ID - a chat in which the "Forum" mode is enabled
