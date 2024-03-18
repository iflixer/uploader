package telegram

import (
	"errors"
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
)

const ChanIvanezko = 193157785
const ChanVideo = -4169130454

type Service struct {
	api              *tgbotapi.BotAPI
	telegramApiToken string
}

func (s *Service) Send(ch int64, message string) {
	msg := tgbotapi.NewMessage(ch, message)
	msg.ParseMode = tgbotapi.ModeHTML
	if _, err := s.api.Send(msg); err != nil {
		log.Println("telegram send message error:", err)
	}
}

func (s *Service) Listen() {
	// Create a new UpdateConfig struct with an offset of 0. Offsets are used
	// to make sure Telegram knows we've handled previous values and we don't
	// need them repeated.
	updateConfig := tgbotapi.NewUpdate(0)

	// Tell Telegram we should wait up to 30 seconds on each request for an
	// update. This way we can get information just as quickly as making many
	// frequent requests without having to send nearly as many.
	updateConfig.Timeout = 30

	// Start polling Telegram for updates.
	updates := s.api.GetUpdatesChan(updateConfig)

	// Let's go through each update that we're getting from Telegram.
	for update := range updates {
		// Telegram can send many types of updates depending on what your Bot
		// is up to. We only want to look at messages for now, so we can
		// discard any other updates.
		if update.Message == nil {
			continue
		}

		// Now that we know we've gotten a new message, we can construct a
		// reply! We'll take the Chat ID and Text from the incoming message
		// and use it to create a new message.
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, update.Message.Text)
		// We'll also say that this message is a reply to the previous message.
		// For any other specifications than Chat ID or Text, you'll need to
		// set fields on the `MessageConfig`.
		msg.ReplyToMessageID = update.Message.MessageID

		// Okay, we're sending our message off! We don't care about the message
		// we just sent, so we'll discard it.
		if _, err := s.api.Send(msg); err != nil {
			// Note that panics are a bad way to handle errors. Telegram can
			// have service outages or network errors, you should retry sending
			// messages or more gracefully handle failures.
			log.Println("error sending telegram message:", err)
		}
	}
}

func NewService(telegramApiToken string) (*Service, error) {
	if telegramApiToken == "" {
		return nil, errors.New("cant create telegram client - token is empty")
	}
	api, err := tgbotapi.NewBotAPI(telegramApiToken)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("cant create telegram client: %s", err))
	}

	api.Debug = false
	s := &Service{
		api:              api,
		telegramApiToken: telegramApiToken,
	}

	// go s.Listen()

	return s, nil
}
