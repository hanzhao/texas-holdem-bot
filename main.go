package main

import (
	"log"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	. "github.com/magicae/telegram-bot"
	"github.com/magicae/texas-holdem-bot/config"
	"gopkg.in/redis.v5"
)

// Global database connection
var redisClient *redis.Client

// Mutex for each group
var critialChatMutex map[int64]*sync.Mutex = map[int64]*sync.Mutex{}

func logger(e *Bot, update *Update) error {
	log.Println("Resolve #", update.UpdateID)
	if update.Message != nil {
		log.Println("Incoming message:", update.Message)
	}
	return nil
}

func textMessageHandler(e *Bot, update *Update) error {
	if update.Message != nil {
		if critialChatMutex[update.Message.Chat.ID] == nil {
			critialChatMutex[update.Message.Chat.ID] = &sync.Mutex{}
		}
		go criticalTextMessageHandler(e, update.Message)
	}
	return nil
}

func criticalTextMessageHandler(e *Bot, message *Message) {
	critialChatMutex[message.Chat.ID].Lock()

	var val int64
	var err error

	text := message.Text
	suffix := "@" + config.Bot.Username

	/*
		if (message.Chat.Type == "group" || message.Chat.Type == "supergroup") &&
			message.ReplyToMessage == nil {
			suffix := "@" + config.Bot.Username
			// Ignore commands without @botname in group chat.
			if !strings.HasSuffix(text, suffix) {
				critialChatMutex[message.Chat.ID].Unlock()
				return
			}
			text = strings.TrimSuffix(text, suffix)
		}
	*/
	if strings.HasSuffix(text, suffix) {
		text = strings.TrimSuffix(text, suffix)
	}

	switch text {
	case "/new":
		if message.Chat.Type == "group" ||
			message.Chat.Type == "supergroup" {
			err = handleNewGame(e, message.MessageID, message.Chat,
				message.From)
		}
	case "/join":
		if message.Chat.Type == "group" || message.Chat.Type == "supergroup" {
			err = handleJoin(e, message.MessageID, message.Chat, message.From)
		}
	case "/leave":
		if message.Chat.Type == "group" || message.Chat.Type == "supergroup" {
			err = handleLeave(e, message.MessageID, message.Chat, message.From)
		}
	case "/list":
		if message.Chat.Type == "group" || message.Chat.Type == "supergroup" {
			err = handleList(e, message.MessageID, message.Chat, message.From)
		}
	case "/start":
		if message.Chat.Type == "private" {
			err = handlePrivateStart(e, message.MessageID, message.Chat,
				message.From)
		} else {
			err = handleStartRound(e, message.MessageID, message.Chat,
				message.From)
		}
	case "/call":
		if message.Chat.Type == "group" || message.Chat.Type == "supergroup" {
			err = handleCall(e, message.MessageID, message.Chat, message.From)
		}
	case "/check":
		if message.Chat.Type == "group" || message.Chat.Type == "supergroup" {
			err = handleCheck(e, message.MessageID, message.Chat, message.From)
		}
	case "/fold":
		if message.Chat.Type == "group" || message.Chat.Type == "supergroup" {
			err = handleFold(e, message.MessageID, message.Chat, message.From)
		}
	case "/raise":
		if message.Chat.Type == "group" || message.Chat.Type == "supergroup" {
			err = handleRaise(e, message.MessageID, message.Chat, message.From)
		}
	case "/allin":
		if message.Chat.Type == "group" || message.Chat.Type == "supergroup" {
			err = handleAllIn(e, message.MessageID, message.Chat, message.From)
		}
	case "/getmoney":
		err = handleGetMoney(e, message.MessageID, message.Chat, message.From)
	case "/wallet":
		err = handleWallet(e, message.MessageID, message.Chat, message.From)
	default:
		val, err = strconv.ParseInt(text, 10, 64)
		if err != nil {
			err = nil
		} else {
			err = handleRaiseN(e, val, message.MessageID, message.Chat,
				message.From)
		}
	}
	if err != nil {
		log.Println("Error:", err, "< criticalTextMessage")
	}

	critialChatMutex[message.Chat.ID].Unlock()
}

func main() {
	rand.Seed(time.Now().UnixNano())
	redisClient = redis.NewClient(config.Database)
	e := NewBot(config.Bot.Token)
	me, err := e.GetMe()
	if err != nil {
		panic("Error: " + err.Error())
	} else {
		config.Bot.Username = me.Username
		config.Bot.ID = me.ID
		log.Println("Bot info:", me)
	}
	e.AddHandler(textMessageHandler)
	e.RunLongPolling()
}
