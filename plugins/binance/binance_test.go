package main

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNewBIClient(t *testing.T) {
	client := NewBIClient(&defaultConfig)
	defer client.Close()
	prices, err := client.FetchPrice([]string{"BTCUSD", "ETHUSD"})
	require.NoError(t, err)
	require.Equal(t, 2, len(prices))
}

func TestBIClient_AvailableSymbols(t *testing.T) {
	client := NewBIClient(&defaultConfig)
	defer client.Close()
	symbols, err := client.AvailableSymbols()
	require.NoError(t, err)

	require.Contains(t, symbols, "BTCUSD")
}
