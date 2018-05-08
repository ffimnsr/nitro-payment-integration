package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPing(t *testing.T) {
	stat, err := ping()
	assert.True(t, stat)
	assert.Nil(t, err)
}

func TestExchangeRates(t *testing.T) {
}
