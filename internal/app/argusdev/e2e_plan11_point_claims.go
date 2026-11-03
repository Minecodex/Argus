package argusdev

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const plan11ClaimNumber = `([-+]?(?:[0-9][0-9,]*(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][-+]?[0-9]+)?)`
const plan11ClaimClock = `([0-2][0-9]:[0-5][0-9]:[0-5][0-9])(?:\.[0-9]+)?Z?`

var plan11ClockThenValue = regexp.MustCompile(plan11ClaimClock + `\)?\s*(?:=|:|为)\s*[≈~]?\s*` + plan11ClaimNumber + `\s*(%|GiB|By)?`)
var plan11ClockParenthesizedValue = regexp.MustCompile(plan11ClaimClock + `\s*[（(]\s*[≈~]?\s*` + plan11ClaimNumber + `\s*(%|GiB|By)?\s*[)）]`)
var plan11ValueAtClock = regexp.MustCompile(plan11ClaimNumber + `\s*(%|GiB|By)?\s*[)）]?\s+at\s+(?:[0-9]{4}-[0-9]{2}-[0-9]{2}T)?` + plan11ClaimClock)

// This is a bounded check of explicit point-value associations in a report
// about one original metric. It is not a general prose or label oracle; mixed
// dashboard/metric narratives require their own contexts and manual review.
func plan11VerifyExplicitPointClaims(body []byte, conclusion plan11Conclusion, texts ...string) error {
	var original struct {
		Data struct {
			Rows []struct {
				Labels map[string]string   `json:"metric"`
				Values [][]json.RawMessage `json:"values"`
				Value  []json.RawMessage   `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &original) != nil {
		return errors.New("point-claim original is invalid")
	}
	byClock := map[string][]float64{}
	for _, row := range original.Data.Rows {
		if row.Labels["__name__"] != conclusion.Metric {
			return errors.New("point-claim check needs one original metric population")
		}
		points := row.Values
		if len(row.Value) != 0 {
			points = append(points, row.Value)
		}
		for _, point := range points {
			var seconds float64
			var text string
			if len(point) != 2 || json.Unmarshal(point[0], &seconds) != nil || json.Unmarshal(point[1], &text) != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
				return errors.New("point-claim original point is invalid")
			}
			value, err := strconv.ParseFloat(text, 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				return errors.New("point-claim original value is invalid")
			}
			clock := time.UnixMilli(int64(math.Round(seconds * 1000))).UTC().Format("15:04:05")
			byClock[clock] = append(byClock[clock], value)
		}
	}
	if len(byClock) == 0 {
		return errors.New("point-claim original has no points")
	}
	check := func(clock, text, unit string) error {
		text = strings.ReplaceAll(text, ",", "")
		claimed, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(claimed) || math.IsInf(claimed, 0) {
			return errors.New("point claim has an invalid number")
		}
		for _, raw := range byClock[clock] {
			value := raw
			switch unit {
			case "":
			case "%":
				if conclusion.Unit == "1" && plan11DisplayTransform(conclusion.Transform) == "percent" {
					value = raw * 100
				} else if conclusion.Unit == "1" && plan11DisplayTransform(conclusion.Transform) == "invert_percent" {
					value = (1 - raw) * 100
				} else if conclusion.Unit != "%" {
					continue
				}
			case "GiB":
				if conclusion.Unit == "By" {
					value = raw / math.Pow(2, 30)
				} else if conclusion.Unit != "GiB" {
					continue
				}
			case "By":
				if conclusion.Unit != "By" {
					continue
				}
			}
			if math.Abs(claimed-value) <= plan11ClaimRoundingTolerance(text, value) {
				return nil
			}
		}
		return fmt.Errorf("explicit point claim %s%s at %s differs from original values", text, unit, clock)
	}
	for _, text := range texts {
		for _, pattern := range []*regexp.Regexp{plan11ClockThenValue, plan11ClockParenthesizedValue} {
			for _, match := range pattern.FindAllStringSubmatch(text, -1) {
				if err := check(match[1], match[2], match[3]); err != nil {
					return err
				}
			}
		}
		for _, match := range plan11ValueAtClock.FindAllStringSubmatch(text, -1) {
			if err := check(match[3], match[1], match[2]); err != nil {
				return err
			}
		}
	}
	return nil
}

func plan11ClaimRoundingTolerance(text string, value float64) float64 {
	parts := strings.Split(strings.ToLower(text), "e")
	exponent := 0
	if len(parts) == 2 {
		exponent, _ = strconv.Atoi(parts[1])
	}
	digits := 0
	if dot := strings.Index(parts[0], "."); dot >= 0 {
		digits = len(parts[0]) - dot - 1
	}
	return 0.5*math.Pow(10, float64(exponent-digits))*(1+1e-9) + math.Max(1e-12, math.Abs(value)*1e-12)
}
