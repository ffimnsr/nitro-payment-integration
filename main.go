package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"

	_ "github.com/lib/pq"
)

const (
	CoinGateLiveURL    string = "https://api.coingate.com/v2"
	CoinGateSandboxURL string = "https://api-sandbox.coingate.com/v2"
)

func main() {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	connStr := "postgres://sesame:secret@localhost/sesame?sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}

	ping()

	e.GET("/", hello)
	e.Logger.Fatal(e.Start("127.0.0.1:1313"))
}

func hello(c echo.Context) error {
	return c.String(http.StatusOK, "hello, world!")
}

func callback(c echo.Context) error {
	return nil
}

func createOrder(orderID string, amount float64, priceCurrency, receiveCurrency,
	title, description, callbackURL, successURL, token string) {
	path := fmt.Sprintf("%s/orders", CoinGateSandboxURL)
	log.Print(path)

	headers := map[string]string{
		"Authorization": "Token shT1zpsLRjaD63VsLEGz2Bxmyich_ZQAq5HQip-p",
		"Content-Type":  "application/x-www-form-urlencoded",
	}

	var data struct {
		ID              int64     `json:"id"`
		Status          string    `json:"status"`
		PriceCurrency   string    `json:"price_currency"`
		PriceAmount     string    `json:"price_amount"`
		ReceiveCurrency string    `json:"receive_currency"`
		ReceiveAmount   string    `json:"receive_amount"`
		CreatedAt       time.Time `json:"created_at"`
		OrderID         string    `json:"order_id"`
		PaymentURL      string    `json:"payment_url"`
		Token           string    `json:"token"`
	}

	err := sendPayload("GET", path, headers, nil, &data)
	if err != nil {
		log.Fatal(err)
	}

	log.Print(data)
}

func getExchangeRates(from, to string) {
	path := fmt.Sprintf("%s/rates/merchant/%s/%s", CoinGateSandboxURL, from, to)
	log.Print(path)

	var data string
	err := sendPayload("GET", path, nil, nil, &data)
	if err != nil {
		log.Fatal(err)
	}

	log.Print(data)
}

func ping() (bool, error) {
	path := fmt.Sprintf("%s/ping", CoinGateSandboxURL)
	log.Print(path)

	var data struct {
		Ping string
		Time time.Time
	}

	err := sendPayload("GET", path, nil, nil, &data)
	if err != nil {
		return false, err
	}

	if data.Ping == "pong" {
		return true, nil
	}

	return false, err
}

func sendPayload(method, path string, headers map[string]string, body io.Reader,
	result interface{}) error {
	method = strings.ToUpper(method)

	req, err := http.NewRequest(method, path, body)
	if err != nil {
		return err
	}

	for k, v := range headers {
		req.Header.Add(k, v)
	}

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	contents, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	log.Printf("-- %d", res.StatusCode)

	if err := json.Unmarshal(contents, &result); err != nil {
		result = string(contents)
	}

	return nil
}
