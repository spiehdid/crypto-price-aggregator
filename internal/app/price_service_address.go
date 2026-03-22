package app

import (
	"context"
	"strings"

	"github.com/spiehdid/crypto-price-aggregator/internal/domain/model"
)

type AddressPriceResult struct {
	Price       *model.Price
	ResolvedVia string // "registry", "lazy", "direct"
}

func (s *PriceService) GetPriceByAddress(ctx context.Context, chain, address, currency string) (*AddressPriceResult, error) {
	address = strings.ToLower(address)
	chain = strings.ToLower(chain)

	if s.registry != nil {
		if coinID, found := s.registry.Resolve(chain, address); found {
			price, err := s.GetPrice(ctx, coinID, currency)
			if err != nil {
				return nil, err
			}
			return &AddressPriceResult{Price: price, ResolvedVia: "registry"}, nil
		}
	}

	for _, tp := range s.tokenProviders {
		if !tp.SupportsChain(chain) {
			continue
		}
		price, err := tp.GetPriceByAddress(ctx, chain, address, currency)
		if err != nil {
			continue
		}
		if s.registry != nil {
			s.registry.Add(chain, address, price.CoinID)
		}
		return &AddressPriceResult{Price: price, ResolvedVia: "direct"}, nil
	}

	return nil, model.ErrCoinNotFound
}
