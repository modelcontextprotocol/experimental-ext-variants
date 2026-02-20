package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetWeatherInput struct {
	Location string `json:"location" jsonschema:"location name or coordinates (e.g. 'San Francisco, CA' or '37.7749,-122.4194')"`
	Units    string `json:"units,omitempty" jsonschema:"temperature units: celsius, fahrenheit (default: celsius)"`
}

type GetWeatherOutput struct {
	Location    string  `json:"location"`
	Temperature float64 `json:"temperature"`
	Units       string  `json:"units"`
	Condition   string  `json:"condition"`
	Humidity    int     `json:"humidity"`
	WindSpeed   float64 `json:"windSpeed"`
}

type GetForecastInput struct {
	Location string `json:"location" jsonschema:"location name or coordinates"`
	Days     int    `json:"days,omitempty" jsonschema:"number of days to forecast (1-7, default: 3)"`
	Units    string `json:"units,omitempty" jsonschema:"temperature units: celsius, fahrenheit (default: celsius)"`
}

type ForecastDay struct {
	Date      string  `json:"date"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Condition string  `json:"condition"`
}

type GetForecastOutput struct {
	Location string        `json:"location"`
	Units    string        `json:"units"`
	Forecast []ForecastDay `json:"forecast"`
}

func getWeather(_ context.Context, _ *mcp.CallToolRequest, in GetWeatherInput) (*mcp.CallToolResult, GetWeatherOutput, error) {
	units := in.Units
	if units == "" {
		units = "celsius"
	}
	temp := 22.5
	if units == "fahrenheit" {
		temp = 72.5
	}
	return nil, GetWeatherOutput{
		Location:    in.Location,
		Temperature: temp,
		Units:       units,
		Condition:   "partly cloudy",
		Humidity:    65,
		WindSpeed:   12.3,
	}, nil
}

func getForecast(_ context.Context, _ *mcp.CallToolRequest, in GetForecastInput) (*mcp.CallToolResult, GetForecastOutput, error) {
	units := in.Units
	if units == "" {
		units = "celsius"
	}
	days := in.Days
	if days == 0 {
		days = 3
	}
	forecast := make([]ForecastDay, days)
	conditions := []string{"sunny", "partly cloudy", "cloudy", "light rain", "clear", "overcast", "scattered showers"}
	for i := range forecast {
		high := 24.0 + float64(i)
		low := 14.0 + float64(i)
		if units == "fahrenheit" {
			high = 75.0 + float64(i)*2
			low = 57.0 + float64(i)*2
		}
		forecast[i] = ForecastDay{
			Date:      "2025-01-0" + string(rune('1'+i)),
			High:      high,
			Low:       low,
			Condition: conditions[i%len(conditions)],
		}
	}
	return nil, GetForecastOutput{
		Location: in.Location,
		Units:    units,
		Forecast: forecast,
	}, nil
}
