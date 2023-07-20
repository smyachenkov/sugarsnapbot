package main

import (
	"go.uber.org/zap"
	"os"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/sashabaranov/go-openai"
)

var (
	MessageGreeting = "Hello! Send me a recipe text."
)

var Logger zap.SugaredLogger

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	Logger = *logger.Sugar()

	// check all env variables
	envOpenaiToken := os.Getenv("OPENAI_TOKEN")
	envTelegramToken := os.Getenv("BOT_TOKEN")
	envNutritionixAPIkey := os.Getenv("NUTRITIONIX_API_KEY")
	envNutritionixAppID := os.Getenv("NUTRITIONIX_APP_ID")

	ingredientsParser := IngredientsParser{
		client: *openai.NewClient(envOpenaiToken),
	}
	nutritionChecker := NutritionChecker{
		nutritionixAPIkey: envNutritionixAPIkey,
		nutritionixAppID:  envNutritionixAppID,
	}
	nutritionCalculator := RecipeNutritionCalculator{
		ingredientsParser: ingredientsParser,
		nutritionChecker:  nutritionChecker,
	}

	bot, err := tele.NewBot(tele.Settings{
		Token:  envTelegramToken,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		Logger.Fatal(err)
		return
	}

	bot.Handle("/start", func(c tele.Context) error {
		return c.Send(MessageGreeting)
	})

	bot.Handle(tele.OnText, func(c tele.Context) error {
		_ = c.Sender()
		text := c.Text()

		// validate
		if len(text) < 10 {
			return c.Send("This doesn't look like a recipe, try againg and provide more data")
		}

		// respond immediately
		err = c.Send("Analyzing the recipe! Will come back with the result soon.")
		if err != nil {
			Logger.Errorf("Failed to send a message: %w", err)
			return nil
		}

		// calculate result
		go func() {
			err = nutritionCalculator.calculateRecipeCarbs(c, text)
			if err != nil {
				Logger.Error("Failed to analyze the recipe request, {}", err)
				err = c.Send("Sorry, can't analyze it.")
				if err != nil {
					Logger.Errorf("Failed to send a message: %w", err)
				}
			}
		}()
		return nil
	})
	bot.Start()
}

type RecipeNutritionCalculator struct {
	ingredientsParser IngredientsParser
	nutritionChecker  NutritionChecker
}

func (n RecipeNutritionCalculator) calculateRecipeCarbs(c tele.Context, text string) error {
	ingredients, err := n.ingredientsParser.getRecipeIngredients(text)
	if err != nil {
		Logger.Errorf("Error: %w", err)
	}
	result, err := n.nutritionChecker.getNutrition(ingredients)
	if err != nil {
		Logger.Errorf("Error: %w", err)
	}
	respText := result.toTelegramResponse()
	return c.Send(respText, tele.ModeMarkdownV2)
}
