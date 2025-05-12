package notification

import (
	"context"
	"fmt"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/handler"
	"remnawave-tg-shop-bot/internal/translation"
	"time"
)

type SubscriptionService struct {
	customerRepository *database.CustomerRepository
	telegramBot        *bot.Bot
	tm                 *translation.Manager
}

func NewSubscriptionService(customerRepository *database.CustomerRepository, telegramBot *bot.Bot, tm *translation.Manager) *SubscriptionService {
	return &SubscriptionService{customerRepository: customerRepository, telegramBot: telegramBot, tm: tm}
}

func (s *SubscriptionService) SendSubscriptionNotifications(ctx context.Context) error {
	customers, err := s.getCustomersWithExpiringSubscriptions()
	if err != nil {
		return fmt.Errorf("failed to get customers with expiring subscriptions: %w", err)
	}

	slog.Info(fmt.Sprintf("Found %d customers with expiring subscriptions", len(*customers)))

	now := time.Now()
	for _, customer := range *customers {

		daysUntilExpiration := s.getDaysUntilExpiration(now, *customer.ExpireAt)

		err := s.sendNotification(ctx, customer)
		if err != nil {
			slog.Error("Failed to send notification",
				"customer_id", customer.ID,
				"days_until_expiration", daysUntilExpiration,
				"error", err)
			continue
		}

		slog.Info("Notification sent successfully",
			"customer_id", customer.ID,
			"days_until_expiration", daysUntilExpiration)
	}

	return nil
}

func (s *SubscriptionService) getCustomersWithExpiringSubscriptions() (*[]database.Customer, error) {
	now := time.Now()
	endDate := now.AddDate(0, 0, 3)

	dbCustomers, err := s.customerRepository.FindByExpirationRange(context.Background(), now, endDate)
	if err != nil {
		return nil, err
	}

	return dbCustomers, nil
}

func (s *SubscriptionService) getDaysUntilExpiration(now time.Time, expireAt time.Time) int {
	nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	expireDate := time.Date(expireAt.Year(), expireAt.Month(), expireAt.Day(), 0, 0, 0, 0, expireAt.Location())

	duration := expireDate.Sub(nowDate)
	return int(duration.Hours() / 24)
}

func (s *SubscriptionService) sendNotification(ctx context.Context, customer database.Customer) error {
	expireDate := customer.ExpireAt.Format("02.01.2006")

	messageText := fmt.Sprintf(
		s.tm.GetText(customer.Language, "subscription_expiring"),
		expireDate,
	)

	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    customer.TelegramID,
		Text:      messageText,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{
						Text:         s.tm.GetText(customer.Language, "renew_subscription_button"),
						CallbackData: handler.CallbackBuy,
					},
				},
			},
		},
	})

	return err
}
