package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Конфигурация, загружаемая из окружения
var (
	telegramBotToken    string
	cleverTapAccountID  string
	cleverTapPasscode   string
	cleverTapAPIBaseURL = "https://api.clevertap.com/1/upload" // Измените на eu1 и т.д., если ваш регион другой
)

// Кнопки на клавиатуре
const (
	btnText    = "Текст"
	btnPicture = "Картинка"
	btnVideo   = "Видео"
)

// --- Структуры для CleverTap API ---

// CTPayload - общая обертка для запросов к CleverTap
type CTPayload struct {
	Data []interface{} `json:"d"`
}

// CTProfileData - данные для профиля пользователя
type CTProfileData struct {
	Type        string      `json:"type"`
	Identity    string      `json:"identity"` // Используем Telegram ID
	ProfileData CTUserProps `json:"profileData"`
}

// CTUserProps - кастомные поля профиля
type CTUserProps struct {
	Name        string `json:"Name"` // Стандартное поле CleverTap
	TGUsername  string `json:"tg_username"`
	TGChatID    int64  `json:"tg_chat_id"`
	TGFirstName string `json:"tg_first_name"`
	TGLastName  string `json:"tg_last_name"`
}

// CTEventData - данные для события
type CTEventData struct {
	Type     string      `json:"type"`
	Identity string      `json:"identity"` // Telegram ID
	EvtName  string      `json:"evtName"`
	EvtData  CTEventProps `json:"evtData"`
}

// CTEventProps - свойства события
type CTEventProps struct {
	TGChatID  int64  `json:"tg_chat_id"`
	Source    string `json:"source"`
}

// --- ---

func main() {
	// 1. Загрузка конфигурации
	telegramBotToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	cleverTapAccountID = os.Getenv("CLEVERTAP_ACCOUNT_ID")
	cleverTapPasscode = os.Getenv("CLEVERTAP_ACCOUNT_PASSCODE")

	if telegramBotToken == "" || cleverTapAccountID == "" || cleverTapPasscode == "" {
		log.Fatal("Ошибка: Переменные окружения TELEGRAM_BOT_TOKEN, CLEVERTAP_ACCOUNT_ID, и CLEVERTAP_ACCOUNT_PASSCODE должны быть установлены.")
	}

	// 2. Инициализация Telegram Bot API
	bot, err := tgbotapi.NewBotAPI(telegramBotToken)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true
	log.Printf("Авторизован как %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	// 3. Главный цикл обработки сообщений
	for update := range updates {
		if update.Message == nil { // Игнорируем другие типы (напр. callback_query)
			continue
		}

		// Данные пользователя для идентификации
		user := update.Message.From
		chatID := update.Message.Chat.ID
		// Конвертируем Telegram User ID (int64) в строку для CleverTap 'identity'
		identity := strconv.FormatInt(user.ID, 10) 

		log.Printf("[%s] (%d) %s", user.UserName, chatID, update.Message.Text)

		// 4. Обработка команд и кнопок
		switch update.Message.Text {
		case "/start":
			// 4.1. Новый пользователь -> Отправляем профиль в CleverTap
			err := uploadCleverTapProfile(user, chatID)
			if err != nil {
				log.Printf("Ошибка отправки профиля в CleverTap: %v", err)
				// (Опционально) можно отправить пользователю сообщение об ошибке
			}

			// 4.2. Отправляем приветственное сообщение с кнопками
			sendWelcomeKeyboard(bot, chatID)

		case btnText, btnPicture, btnVideo:
			// 4.3. Пользователь нажал кнопку -> Отправляем событие в CleverTap
			var eventName string
			switch update.Message.Text {
			case btnText:
				eventName = "TestWelcomeText"
			case btnPicture:
				eventName = "TestWelcomePicture"
			case btnVideo:
				eventName = "TestWelcomeVideo"
			}

			err := pushCleverTapEvent(identity, chatID, eventName)
			if err != nil {
				log.Printf("Ошибка отправки события в CleverTap: %v", err)
			}
			
			// Отвечаем пользователю
			reply := fmt.Sprintf("Событие '%s' отправлено!", eventName)
			msg := tgbotapi.NewMessage(chatID, reply)
			bot.Send(msg)

		default:
			// 4.4. Неизвестная команда
			msg := tgbotapi.NewMessage(chatID, "Используйте кнопки ниже или команду /start")
			bot.Send(msg)
		}
	}
}

// sendWelcomeKeyboard отправляет приветственное сообщение и клавиатуру
func sendWelcomeKeyboard(bot *tgbotapi.BotAPI, chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Добро пожаловать! Выберите опцию:")
	
	// Создаем клавиатуру
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(btnText),
			tgbotapi.NewKeyboardButton(btnPicture),
			tgbotapi.NewKeyboardButton(btnVideo),
		),
	)
	keyboard.ResizeKeyboard = true // Делает кнопки удобнее
	
	msg.ReplyMarkup = keyboard

	if _, err := bot.Send(msg); err != nil {
		log.Printf("Ошибка отправки клавиатуры: %v", err)
	}
}

// uploadCleverTapProfile создает/обновляет профиль пользователя в CleverTap
func uploadCleverTapProfile(user *tgbotapi.User, chatID int64) error {
	log.Printf("Отправка профиля для ID: %d", user.ID)

	// Собираем профиль
	profile := CTProfileData{
		Type:     "profile",
		Identity: strconv.FormatInt(user.ID, 10), // Ключ 'identity'
		ProfileData: CTUserProps{
			Name:        user.FirstName + " " + user.LastName, // Полное имя
			TGUsername:  user.UserName,
			TGChatID:    chatID,
			TGFirstName: user.FirstName,
			TGLastName:  user.LastName,
		},
	}

	// Оборачиваем в 'd'
	payload := CTPayload{
		Data: []interface{}{profile},
	}

	return sendToCleverTap(payload)
}

// pushCleverTapEvent отправляет кастомное событие в CleverTap
func pushCleverTapEvent(identity string, chatID int64, eventName string) error {
	log.Printf("Отправка события '%s' для ID: %s", eventName, identity)
	
	event := CTEventData{
		Type:     "event",
		Identity: identity,
		EvtName:  eventName,
		EvtData: CTEventProps{
			TGChatID: chatID,
			Source:   "TonWelcomeBot",
		},
	}

	payload := CTPayload{
		Data: []interface{}{event},
	}
	
	return sendToCleverTap(payload)
}

// sendToCleverTap - универсальный HTTP-клиент для отправки данных
func sendToCleverTap(payload CTPayload) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("ошибка маршалинга JSON: %w", err)
	}

	req, err := http.NewRequest("POST", cleverTapAPIBaseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("ошибка создания HTTP-запроса: %w", err)
	}

	// Установка обязательных заголовков
	req.Header.Set("X-CleverTap-Account-Id", cleverTapAccountID)
	req.Header.Set("X-CleverTap-Passcode", cleverTapPasscode)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка выполнения HTTP-запроса: %w", err)
	}
	defer resp.Body.Close()

	// Проверка статуса ответа
	if resp.StatusCode != http.StatusOK {
		// (Здесь можно добавить парсинг тела ответа, чтобы увидеть ошибку от CleverTap)
		return fmt.Errorf("неуспешный статус от CleverTap: %s", resp.Status)
	}

	log.Println("Данные успешно отправлены в CleverTap.")
	return nil
}