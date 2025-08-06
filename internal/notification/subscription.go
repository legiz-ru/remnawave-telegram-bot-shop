package notification

import (
	"context"
	"fmt"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/handler"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/translation"
	"time"
)

type SubscriptionService struct {
	customerRepository *database.CustomerRepository
	purchaseRepository *database.PurchaseRepository
	paymentService     *payment.PaymentService
	telegramBot        *bot.Bot
	tm                 *translation.Manager
}

func NewSubscriptionService(customerRepository *database.CustomerRepository,
	purchaseRepository *database.PurchaseRepository,
	paymentService *payment.PaymentService,
	telegramBot *bot.Bot,
	tm *translation.Manager) *SubscriptionService {
	return &SubscriptionService{customerRepository: customerRepository, purchaseRepository: purchaseRepository, paymentService: paymentService, telegramBot: telegramBot, tm: tm}
}
func (s *SubscriptionService) ProcessSubscriptionExpiration() error {
	ctx := context.Background()
	customers, err := s.getCustomersWithExpiringSubscriptions()
	if err != nil {
		slog.Error("Failed to get customers with expiring subscriptions", "error", err)
		return err
	}

	slog.Info(fmt.Sprintf("Found %d customers with expiring subscriptions", len(*customers)))
	if len(*customers) == 0 {
		return nil
	}
	now := time.Now()

	customersIds := make([]int64, len(*customers))
	for i, customer := range *customers {
		customersIds[i] = customer.ID
	}

	nonCancelledTributes, err := s.purchaseRepository.FindTributesByCustomerIDs(ctx, customersIds)
	if err != nil {
		slog.Error("Failed to query tribute purchases", "error", err)
		return err
	}

	customerIdTributes := make(map[int64]*database.Purchase, len(*nonCancelledTributes))
	for i := range *nonCancelledTributes {
		p := &(*nonCancelledTributes)[i]
		customerIdTributes[p.CustomerID] = p
	}

	tributesProcessed := make(map[int64]bool, len(*nonCancelledTributes))

	for _, customer := range *customers {
		daysUntilExpiration := s.getDaysUntilExpiration(now, *customer.ExpireAt)

		if p, ok := customerIdTributes[customer.ID]; ok {
			if daysUntilExpiration != 1 {
				continue
			}
			_, purchaseId, err := s.paymentService.CreatePurchase(ctx, p.Amount, p.Month, &customer, database.InvoiceTypeTribute)
			if err != nil {
				slog.Error("Failed to create tribute purchase", "error", err)
				continue
			}

			err = s.paymentService.ProcessPurchaseById(ctx, purchaseId)
			if err != nil {
				slog.Error("Failed to process tribute purchase", "error", err)
				continue
			}
			slog.Info("Tribute purchase processed successfully", "purchase_id", purchaseId)
			tributesProcessed[customer.ID] = true
		}
		if _, ok := tributesProcessed[customer.ID]; ok {
			continue
		}

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

	slog.Info(fmt.Sprintf("Processed tributes customers %d with expiring subscriptions", len(tributesProcessed)))
	slog.Info(fmt.Sprintf("Sent notifications to %d customers with expiring subscriptions", len(*customers)-len(tributesProcessed)))
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
