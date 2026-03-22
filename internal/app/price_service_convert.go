package app

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

type ConvertResult struct {
	From        string
	To          string
	Amount      decimal.Decimal
	Result      decimal.Decimal
	Rate        decimal.Decimal
	ViaCurrency string
}

func (s *PriceService) Convert(ctx context.Context, fromCoin, toCoin string, amount decimal.Decimal, viaCurrency string) (*ConvertResult, error) {
	fromPrice, err := s.GetPrice(ctx, fromCoin, viaCurrency)
	if err != nil {
		return nil, fmt.Errorf("getting %s price: %w", fromCoin, err)
	}

	toPrice, err := s.GetPrice(ctx, toCoin, viaCurrency)
	if err != nil {
		return nil, fmt.Errorf("getting %s price: %w", toCoin, err)
	}

	if fromPrice.Value.IsZero() {
		return nil, fmt.Errorf("price for %s is zero", fromCoin)
	}
	if toPrice.Value.IsZero() {
		return nil, fmt.Errorf("price for %s is zero", toCoin)
	}

	rate := fromPrice.Value.Div(toPrice.Value)
	result := amount.Mul(rate)

	return &ConvertResult{
		From:        fromCoin,
		To:          toCoin,
		Amount:      amount,
		Result:      result,
		Rate:        rate,
		ViaCurrency: viaCurrency,
	}, nil
}
