package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/sashabaranov/go-openai"
)

const (
	ParserPrompt = `I will give you a dish recipe.
 		 Provide a list of ingredients of this dish.
		 If you found zero ingredients or text is unrelated to recipes - respond with "NO_INGREDIENTS" text.
		 Try to provide generic name for every ingredient in the the genericName field. Generic name is the name without any brands.
		 Convert every possible measurement to grams.
         Parse every ingredient into csv line with fields:
		 name, generic_name, quantity_grams, original_quantity, original_quantity_unit. 
		 name field is the original input name of the ingredient. 
		 generic_name field is the generic of the ingredient. 
		 quantity_grams field is the amount of ingredient in grams. If unable to calculate quantity_grams - use "NO_QUANTITY" value 
		 original_quantity field is the amount of ingredient in original units. 
		 original_quantity_unit in the original units if they are different from grams.
		 original_quantity and original_quantity_units are filled only if this ingredient was converted to grams
		 Return csv data.`
)

type IngredientsParser struct {
	client openai.Client
}

type Ingredient struct {
	Name                 string  `json:"name"`
	GenericName          string  `json:"genericName,omitempty"`
	QuantityGrams        float64 `json:"quantityGrams"`
	OriginalQuantity     string  `json:"originalQuantity,omitempty"`
	OriginalQuantityUnit string  `json:"originalQuantityUnit,omitempty"`
}

func (p IngredientsParser) getRecipeIngredients(text string) ([]Ingredient, error) {
	result := []Ingredient{}
	Logger.Infof("Summarizing text %s", text)

	resp, err := p.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:            openai.GPT3Dot5Turbo,
			TopP:             1,
			Temperature:      0.5,
			MaxTokens:        1000,
			FrequencyPenalty: 0,
			PresencePenalty:  0,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: ParserPrompt,
				}, {
					Role:    openai.ChatMessageRoleUser,
					Content: text,
				},
			},
		},
	)
	if err != nil {
		Logger.Errorf("ChatCompletion error %s", err.Error())
		return result, err
	}
	Logger.Infof("Parsed ingredients %v", resp.Choices)

	choice := resp.Choices[0]
	if choice.Message.Content == "NO_INGREDIENTS" {
		return result, fmt.Errorf("no ingridients in recipe")
	}

	// parse into ingredients
	r := csv.NewReader(strings.NewReader(choice.Message.Content))
	// skip header
	_, err = r.Read()
	if err != nil {
		Logger.Errorf("Failed to read csv header %s", err.Error())
		return result, err
	}

	ingredientMap := map[string][]Ingredient{}

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			Logger.Errorf("Failed to parse ingredient from row %w, skipping %v", record, err)
			continue
		}

		grams, err := strconv.ParseFloat(record[2], 64)
		if err != nil {
			Logger.Errorf("Failed to parse grams for %v", record)
			grams = 0
		}
		grams = grams * 100 / 100

		ingredient := Ingredient{
			Name:                 record[0],
			GenericName:          record[1],
			QuantityGrams:        grams,
			OriginalQuantity:     record[2],
			OriginalQuantityUnit: record[3],
		}
		ingredientMap[record[1]] = append(ingredientMap[record[1]], ingredient)
	}

	// merge same ingredients
	for name, ingredients := range ingredientMap {
		if len(ingredients) > 1 {
			Logger.Infof("Found multiple entries for %s, merging %v", name, ingredients)
			merged := Ingredient{
				Name:          ingredients[0].Name,
				GenericName:   ingredients[0].GenericName,
				QuantityGrams: 0,
				// skip original quantities since they can be different
				OriginalQuantity:     "-",
				OriginalQuantityUnit: "-",
			}
			for _, ing := range ingredients {
				merged.QuantityGrams += ing.QuantityGrams
			}
			Logger.Infof("Merge result: %v", merged)
			result = append(result, merged)
		} else {
			result = append(result, ingredients[0])
		}
	}
	return result, nil
}
