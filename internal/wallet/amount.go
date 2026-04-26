package wallet

import (
	"fmt"
	"math/big"
	"strings"
)

var stroopMultiplier = big.NewInt(10_000_000)

func ParseAmountToStroops(raw string) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, ErrInvalidAmount
	}

	if strings.HasPrefix(value, "+") {
		value = strings.TrimPrefix(value, "+")
	}

	if strings.HasPrefix(value, "-") {
		return 0, ErrInvalidAmount
	}

	parts := strings.Split(value, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("%w: malformed decimal", ErrInvalidAmount)
	}

	intPart := parts[0]
	if intPart == "" {
		intPart = "0"
	}

	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
	}

	if len(fracPart) > 7 {
		return 0, fmt.Errorf("%w: max 7 decimal places", ErrInvalidAmount)
	}

	for len(fracPart) < 7 {
		fracPart += "0"
	}

	whole, ok := new(big.Int).SetString(intPart, 10)
	if !ok {
		return 0, fmt.Errorf("%w: malformed integer", ErrInvalidAmount)
	}

	fraction := big.NewInt(0)
	if fracPart != "" {
		var fracOK bool
		fraction, fracOK = new(big.Int).SetString(fracPart, 10)
		if !fracOK {
			return 0, fmt.Errorf("%w: malformed fraction", ErrInvalidAmount)
		}
	}

	total := new(big.Int).Mul(whole, stroopMultiplier)
	total.Add(total, fraction)
	if total.Sign() <= 0 {
		return 0, ErrInvalidAmount
	}
	if !total.IsInt64() {
		return 0, fmt.Errorf("%w: amount too large", ErrInvalidAmount)
	}

	return total.Int64(), nil
}

func FormatStroops(stroops int64) string {
	sign := ""
	if stroops < 0 {
		sign = "-"
		stroops = -stroops
	}

	whole := stroops / 10_000_000
	fraction := stroops % 10_000_000
	return fmt.Sprintf("%s%d.%07d", sign, whole, fraction)
}
