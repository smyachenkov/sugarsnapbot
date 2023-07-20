package main

import (
	"bytes"
	"encoding/json"
	"fmt"

	"io"
	"net/http"
	"strings"
	"time"
)

type NutritionChecker struct {
	nutritionixAPIkey string
	nutritionixAppID  string
}

type NutrientsRequestDTO struct {
	Query               string `json:"query"`
	NumServings         int    `json:"num_servings,omitempty"`
	Aggregate           string `json:"aggregate,omitempty"`
	LineDelimited       bool   `json:"line_delimited,omitempty"`
	UseRawFoods         bool   `json:"use_raw_foods,omitempty"`
	IncludeSubrecipe    bool   `json:"include_subrecipe,omitempty"`
	Timezone            string `json:"timezone,omitempty"`
	ConsumedAt          string `json:"consumed_at,omitempty"`
	Lat                 int    `json:"lat,omitempty"`
	Lng                 int    `json:"lng,omitempty"`
	MealType            int    `json:"meal_type,omitempty"`
	UseBrandedFoods     bool   `json:"use_branded_foods,omitempty"`
	Locale              string `json:"locale,omitempty"`
	Taxonomy            bool   `json:"taxonomy,omitempty"`
	IngredientStatement bool   `json:"ingredient_statement,omitempty"`
	LastModified        bool   `json:"last_modified,omitempty"`
}

type NutrientsResponseDTO struct {
	Foods []FoodDTO `json:"foods"`
}

type FoodDTO struct {
	FoodName            string      `json:"food_name"`
	BrandName           interface{} `json:"brand_name"`
	ServingQty          float64     `json:"serving_qty"`
	ServingUnit         string      `json:"serving_unit"`
	ServingWeightGrams  float64     `json:"serving_weight_grams"`
	NfCalories          float64     `json:"nf_calories"`
	NfTotalFat          float64     `json:"nf_total_fat"`
	NfSaturatedFat      float64     `json:"nf_saturated_fat"`
	NfCholesterol       float64     `json:"nf_cholesterol"`
	NfSodium            float64     `json:"nf_sodium"`
	NfTotalCarbohydrate float64     `json:"nf_total_carbohydrate"`
	NfDietaryFiber      float64     `json:"nf_dietary_fiber"`
	NfSugars            float64     `json:"nf_sugars"`
	NfProtein           float64     `json:"nf_protein"`
	NfPotassium         float64     `json:"nf_potassium"`
	NfP                 float64     `json:"nf_p"`
	FullNutrients       []struct {
		AttrId int     `json:"attr_id"`
		Value  float64 `json:"value"`
	} `json:"full_nutrients"`
	NixBrandName interface{} `json:"nix_brand_name"`
	NixBrandId   interface{} `json:"nix_brand_id"`
	NixItemName  interface{} `json:"nix_item_name"`
	NixItemId    interface{} `json:"nix_item_id"`
	Upc          interface{} `json:"upc"`
	ConsumedAt   time.Time   `json:"consumed_at"`
	Metadata     struct {
		IsRawFood bool `json:"is_raw_food"`
	} `json:"metadata"`
	Source int `json:"source"`
	NdbNo  int `json:"ndb_no"`
	Tags   struct {
		Item      string      `json:"item"`
		Measure   string      `json:"measure"`
		Quantity  interface{} `json:"quantity"`
		FoodGroup int         `json:"food_group"`
		TagId     int         `json:"tag_id"`
	} `json:"tags"`
	AltMeasures []struct {
		ServingWeight float64 `json:"serving_weight"`
		Measure       string  `json:"measure"`
		Seq           *int    `json:"seq"`
		Qty           float64 `json:"qty"`
	} `json:"alt_measures"`
	Lat      interface{} `json:"lat"`
	Lng      interface{} `json:"lng"`
	MealType int         `json:"meal_type"`
	Photo    struct {
		Thumb          string `json:"thumb"`
		Highres        string `json:"highres"`
		IsUserUploaded bool   `json:"is_user_uploaded"`
	} `json:"photo"`
	SubRecipe interface{} `json:"sub_recipe"`
	ClassCode interface{} `json:"class_code"`
	BrickCode interface{} `json:"brick_code"`
	TagId     interface{} `json:"tag_id"`
}

func (c NutritionChecker) getNutrition(ingredients []Ingredient) (RecipeResult, error) {
	result := RecipeResult{
		ingredients: map[string]NutritionSummary{},
	}
	if len(ingredients) == 0 {
		return result, fmt.Errorf("zero ingredients")
	}

	queryParts := []string{}
	for _, ingredient := range ingredients {
		var name string
		if ingredient.GenericName != "" {
			name = ingredient.GenericName
		} else {
			name = ingredient.Name
		}
		queryParts = append(queryParts, name)
	}

	query := strings.Join(queryParts, ", ")
	Logger.Info("Nutritionix query: {}", query)

	nutrionixResp, err := c.getNutritionixResponse(query)
	if err != nil {
		return result, err
	}

	if len(nutrionixResp.Foods) != len(ingredients) {
		return result, fmt.Errorf("couldn't fetch all ingredients from Nutritionix")
	}

	productCarbs := map[string]float64{}
	productWeight := map[string]float64{}
	for i, food := range nutrionixResp.Foods {
		servingGrams := food.ServingWeightGrams
		servingCarbs := food.NfTotalCarbohydrate
		carbsPerGram := servingCarbs / servingGrams

		recipeAmount := ingredients[i].QuantityGrams
		ingredientCarbs := recipeAmount * carbsPerGram

		productCarbs[food.FoodName] = ingredientCarbs
		productWeight[food.FoodName] = recipeAmount

		result.ingredients[food.FoodName] = NutritionSummary{
			weightGrams: recipeAmount,
			carbsGrams:  recipeAmount * carbsPerGram,
		}
	}

	Logger.Info("productWeight {}", productWeight)
	Logger.Info("productCarbs {}", productCarbs)

	totalWeight := float64(0)
	totalCarbs := float64(0)

	for _, v := range result.ingredients {
		totalWeight += v.weightGrams
		totalCarbs += v.carbsGrams
	}

	Logger.Info("Total weight {}", totalWeight)
	Logger.Info("Total carbs {}", totalCarbs)

	result.total = NutritionSummary{
		weightGrams: totalWeight,
		carbsGrams:  totalCarbs,
	}
	return result, nil
}

type RecipeResult struct {
	total       NutritionSummary
	ingredients map[string]NutritionSummary
}

type NutritionSummary struct {
	weightGrams float64
	carbsGrams  float64
}

func (r RecipeResult) toTelegramResponse() string {
	response := "Ingredients:"
	for name, info := range r.ingredients {
		response += fmt.Sprintf("\n · %s: %s grams, %s carbs",
			mdFormatString(name),
			mdFormatFloat(info.weightGrams),
			mdFormatFloat(info.carbsGrams),
		)
	}
	response += "\n\n"

	response += fmt.Sprintf("\n Total weight: %s", mdFormatFloat(r.total.weightGrams))
	response += fmt.Sprintf("\n Total carbs: %s", mdFormatFloat(r.total.carbsGrams))
	Logger.Info("Telegram response: {}", response)
	return response
}

func mdFormatFloat(v float64) string {
	return strings.ReplaceAll(fmt.Sprintf("%.2f", v), ".", "\\.")
}

func mdFormatString(v string) string {
	return strings.ReplaceAll(v, ".", "\\.")
}

func (c NutritionChecker) getNutritionixResponse(query string) (NutrientsResponseDTO, error) {
	url := "https://trackapi.nutritionix.com/v2/natural/nutrients"

	requestDTO := NutrientsRequestDTO{Query: query}
	payload, err := json.Marshal(requestDTO)
	if err != nil {
		Logger.Error("Failed to marshal request DTO {}", requestDTO)
		return NutrientsResponseDTO{}, err
	}
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	req.Header.Add("x-app-id", c.nutritionixAppID)
	req.Header.Add("x-app-key", c.nutritionixAPIkey)
	req.Header.Add("Content-Type", "application/json")

	res, _ := http.DefaultClient.Do(req)

	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)

	var result NutrientsResponseDTO
	err = json.Unmarshal(body, &result)
	if err != nil {
		Logger.Error("Failed to unmarshal response body {}, {}", body, err)
		return NutrientsResponseDTO{}, err
	}

	return result, nil
}
