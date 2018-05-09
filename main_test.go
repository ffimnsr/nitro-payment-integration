package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPing(t *testing.T) {
	res, err := ping()
	assert.Nil(t, err)
	assert.True(t, res)
}

func TestExchangeRates(t *testing.T) {
	res, err := getExchangeRates("BTC", "USD")
	assert.Nil(t, err)
	assert.IsType(t, float64(1), res)
}

func TestCreateOrder(t *testing.T) {
	callbackURL := "https://staging.nitro.live/payments/coingate/callback"
	cancelURL := "https://staging.nitro.live/"
	successURL := "https://staging.nitro.live/success"
	token := "nox-1011"
	res, err := createOrder("1011", 1011.00, "ETH", "USD", "Nitro Order # 1011",
		"Nox Token", callbackURL, cancelURL, successURL, token)
	assert.Nil(t, err)
	assert.IsType(t, CreateOrderResponse{}, res)
	assert.Exactly(t, "new", res.Status)
	assert.Exactly(t, token, res.Token)
}

func TestGetOrder(t *testing.T) {
	res, err := getOrder(84752)
	assert.Nil(t, err)
	assert.Exactly(t, "1011", res.OrderID)
}

func TestListOrders(t *testing.T) {
	res, err := listOrders(2, 1, "created_at_desc")
	assert.Nil(t, err)
	assert.Equal(t, int64(1), res.CurrentPage)
	assert.Equal(t, int64(2), res.PerPage)
}
