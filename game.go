package main

import (
	"fmt"
	"math/rand"
	"strconv"

	. "github.com/magicae/telegram-bot"
	"github.com/magicae/texas-holdem-bot/config"
	"gopkg.in/redis.v5"
)

var games map[int64]*Texas = map[int64]*Texas{}

func handlePrivateStart(e *Bot, id int, chat *Chat, user *User) error {
	chatKey := "texas:user:" + strconv.Itoa(user.ID) + ":chat"
	err := redisClient.Set(chatKey, chat.ID, 0).Err()
	if err != nil {
		return err
	}
	body := &SendMessageRequest{
		ChatID: chat.ID,
		Text:   "You are registered in this bot! Lets start a game in group.",
	}
	_, err = e.SendMessage(body)
	return err
}

func handleNewGame(e *Bot, id int, chat *Chat, user *User) error {
	// The game already started.
	if games[chat.ID] != nil {
		body := &SendMessageRequest{
			ChatID:           chat.ID,
			Text:             "Texas Hold'em has already started.\n/join",
			ReplyToMessageID: id,
		}
		_, err := e.SendMessage(body)
		return err
	}
	// Start a new game.
	// TODO: Make max chip configurable.
	games[chat.ID] = NewTexas(e, chat.ID, 5000)
	// Add the beginner into it.
	chip, err := games[chat.ID].AddUser(user)
	text := ""
	if err != nil {
		games[chat.ID] = nil
		text = "Failed to start a new game. " + err.Error()
	} else {
		text = fmt.Sprintf("%s bought %d chips and started a new game!\n"+
			"/join us to play Texas Hold'em together!",
			getUserDisplayName(user), chip)
	}
	// Start game success.
	body := &SendMessageRequest{
		ChatID:           chat.ID,
		Text:             text,
		ReplyToMessageID: id,
	}
	_, err = e.SendMessage(body)
	return err
}

func handleJoin(e *Bot, id int, chat *Chat, user *User) error {
	// Game is not ready.
	if games[chat.ID] == nil {
		body := &SendMessageRequest{
			ChatID:           chat.ID,
			Text:             "You need to /start Texas Hold'em first!",
			ReplyToMessageID: id,
		}
		_, err := e.SendMessage(body)
		return err
	}
	// Join game.
	chip, err := games[chat.ID].AddUser(user)
	text := ""
	if err != nil {
		text = "Failed to join game. " + err.Error()
	} else {
		text = fmt.Sprintf("%s bought %d chips and joined the game!",
			getUserDisplayName(user), chip)
	}
	// Join game successfully.
	body := &SendMessageRequest{
		ChatID:           chat.ID,
		Text:             text,
		ReplyToMessageID: id,
	}
	_, err = e.SendMessage(body)
	return err
}

func handleList(e *Bot, id int, chat *Chat, user *User) error {
	if games[chat.ID] == nil {
		body := &SendMessageRequest{
			ChatID:           chat.ID,
			Text:             "Game is not ready.",
			ReplyToMessageID: id,
		}
		_, err := e.SendMessage(body)
		return err
	}
	// List user.
	text := ""
	count := 0
	game := games[chat.ID]
	for i := 0; i < 10; i++ {
		if game.Players[i] != nil {
			count++
			text += fmt.Sprintf("[%d] %s - %d\n", count,
				game.Players[i].DisplayName, game.Players[i].Chip)
		}
	}
	text = "Texas Hold'em Players (" + strconv.Itoa(count) + " / 10)\n" + text
	body := &SendMessageRequest{
		ChatID:           chat.ID,
		Text:             text,
		ReplyToMessageID: id,
	}
	_, err := e.SendMessage(body)
	return err
}

func handleLeave(e *Bot, id int, chat *Chat, user *User) error {
	if games[chat.ID] == nil {
		body := &SendMessageRequest{
			ChatID:           chat.ID,
			Text:             "Game is not ready.",
			ReplyToMessageID: id,
		}
		_, err := e.SendMessage(body)
		return err
	}
	if games[chat.ID].Round != nil && games[chat.ID].Round.Stage < End {
		// Try fold.
		games[chat.ID].Fold(user.ID)
		games[chat.ID].GetOut(user.ID)
	}
	chip, err := games[chat.ID].RemoveUser(user)
	text := ""
	if err != nil {
		text = err.Error()
	} else {
		text = "Bye! You took $" + strconv.FormatInt(chip, 10) + " back!"
		if games[chat.ID].CountUser() == 0 {
			games[chat.ID] = nil
			text += " Game ends!"
		}
	}
	body := &SendMessageRequest{
		ChatID:           chat.ID,
		Text:             text,
		ReplyToMessageID: id,
	}
	_, err = e.SendMessage(body)
	return err
}

func handleStartRound(e *Bot, id int, chat *Chat, user *User) error {
	if games[chat.ID] == nil {
		body := &SendMessageRequest{
			ChatID:           chat.ID,
			Text:             "Game is not ready.",
			ReplyToMessageID: id,
		}
		_, err := e.SendMessage(body)
		return err
	}
	err := games[chat.ID].StartRound()
	if err != nil {
		body := &SendMessageRequest{
			ChatID:           chat.ID,
			Text:             err.Error(),
			ReplyToMessageID: id,
		}
		_, err := e.SendMessage(body)
		return err
	}
	return games[chat.ID].MoveOn()
}

func handleFold(e *Bot, id int, chat *Chat, user *User) error {
	game := games[chat.ID]
	if game != nil && game.Round != nil && game.Round.Stage < End &&
		game.Players[game.Round.ActorIndex].UserID == user.ID {
		err := game.Fold(user.ID)
		if err != nil {
			body := &SendMessageRequest{
				ChatID:           chat.ID,
				Text:             err.Error(),
				ReplyToMessageID: id,
			}
			_, err := e.SendMessage(body)
			return err
		}
	}
	return nil
}

func handleCall(e *Bot, id int, chat *Chat, user *User) error {
	game := games[chat.ID]
	if game != nil && game.Round != nil && game.Round.Stage < End &&
		game.Players[game.Round.ActorIndex].UserID == user.ID {
		err := game.Call(user.ID)
		if err != nil {
			body := &SendMessageRequest{
				ChatID:           chat.ID,
				Text:             err.Error(),
				ReplyToMessageID: id,
			}
			_, err := e.SendMessage(body)
			return err
		}
	}
	return nil
}

func handleCheck(e *Bot, id int, chat *Chat, user *User) error {
	game := games[chat.ID]
	if game != nil && game.Round != nil && game.Round.Stage < End &&
		game.Players[game.Round.ActorIndex].UserID == user.ID {
		err := game.Check(user.ID)
		if err != nil {
			body := &SendMessageRequest{
				ChatID:           chat.ID,
				Text:             err.Error(),
				ReplyToMessageID: id,
			}
			_, err := e.SendMessage(body)
			return err
		}
	}
	return nil
}

func handleRaise(e *Bot, id int, chat *Chat, user *User) error {
	game := games[chat.ID]
	if game != nil && game.Round != nil && game.Round.Stage < End &&
		game.Players[game.Round.ActorIndex].UserID == user.ID {
		_, err := e.SendMessage(&SendMessageRequest{
			ChatID:           chat.ID,
			Text:             "How much?",
			ReplyToMessageID: id,
			ReplyMarkup: &ReplyKeyboardMarkup{
				Keyboard:        config.Bot.RaiseButtons,
				Selective:       true,
				OneTimeKeyboard: true,
				ResizeKeyboard:  true,
			},
		})
		return err
	}
	return nil
}

func handleRaiseN(e *Bot, n int64, id int, chat *Chat, user *User) error {
	game := games[chat.ID]
	if game != nil && game.Round != nil && game.Round.Stage < End &&
		game.Players[game.Round.ActorIndex].UserID == user.ID {
		err := game.Raise(user.ID, n)
		if err != nil {
			body := &SendMessageRequest{
				ChatID:           chat.ID,
				Text:             err.Error(),
				ReplyToMessageID: id,
			}
			_, err := e.SendMessage(body)
			return err
		}
	}
	return nil
}

func handleAllIn(e *Bot, id int, chat *Chat, user *User) error {
	game := games[chat.ID]
	if game != nil && game.Round != nil && game.Round.Stage < End &&
		game.Players[game.Round.ActorIndex].UserID == user.ID {
		err := game.AllIn(user.ID)
		if err != nil {
			body := &SendMessageRequest{
				ChatID:           chat.ID,
				Text:             err.Error(),
				ReplyToMessageID: id,
			}
			_, err := e.SendMessage(body)
			return err
		}
	}
	return nil
}

func handleGetMoney(e *Bot, id int, chat *Chat, user *User) error {
	moneyKey := "texas:user:" + strconv.Itoa(user.ID) + ":money"
	money := config.Bot.GetMoneyBase + rand.Int63n(config.Bot.GetMoneyBonus)
	totalMoney, err := redisClient.IncrBy(moneyKey, money).Result()
	if err != nil {
		return err
	}
	body := &SendMessageRequest{
		ChatID: chat.ID,
		Text: fmt.Sprintf("Wow! You got $%d and you have $%d now!", money,
			totalMoney),
		ReplyToMessageID: id,
	}
	_, err = e.SendMessage(body)
	return err
}

func handleWallet(e *Bot, id int, chat *Chat, user *User) error {
	moneyKey := "texas:user:" + strconv.Itoa(user.ID) + ":money"
	money, err := redisClient.Get(moneyKey).Int64()
	if err != nil {
		if err == redis.Nil {
			money = 0
		} else {
			return err
		}
	}
	body := &SendMessageRequest{
		ChatID:           chat.ID,
		Text:             fmt.Sprintf("You have $%d.", money),
		ReplyToMessageID: id,
	}
	_, err = e.SendMessage(body)
	return err
}
