package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"

	_ "github.com/lib/pq"
)

const (
	// API URLs
	PaypalLiveURL      string = "https://api.paypal.com"
	PaypalSandboxURL   string = "https://api.sandbox.paypal.com"
	CoinGateLiveURL    string = "https://api.coingate.com/v2"
	CoinGateSandboxURL string = "https://api-sandbox.coingate.com/v2"

	// API sandbox tokens
	CoingateToken  string = "shT1zpsLRjaD63VsLEGz2Bxmyich_ZQAq5HQip-p"
	PaypalClientID string = "AQU7EFiThYJSVfnMlZvDa8jIC4QYnGOUsGZwHSWHqkMvdRT260mH_8O9M8AYOIhofvsGblCOMuoFZu9u"
	PaypalSecret   string = "EBPQM3M9BJTNL437Oce_JJdKzJYit7tJtfkXqhYq9wSt5Z4LPLHVtwvMz8yO6pTZPnOy2UdrhinckRgL"
)

type (
	Template struct {
		templates *template.Template
	}

	OrderBase struct {
		ID              int64  `json:"id" form:"id"`
		OrderID         string `json:"order_id" form:"order_id"`
		Status          string `json:"status" form:"status"`
		PriceCurrency   string `json:"price_currency" form:"price_currency"`
		PriceAmount     string `json:"price_amount" form:"price_amount"`
		ReceiveCurrency string `json:"receive_currency" form:"receive_currency"`
		ReceiveAmount   string `json:"receive_amount" form:"receive_amount"`
	}

	CreateOrderResponse struct {
		OrderBase
		CreatedAt  time.Time `json:"created_at" form:"created_at"`
		PaymentURL string    `json:"payment_url"`
		Token      string    `json:"token"`
	}

	OrderResponse struct {
		OrderBase
		CreatedAt      time.Time `json:"created_at" form:"created_at"`
		ExpireAt       time.Time `json:"expire_at"`
		PaymentURL     string    `json:"payment_url"`
		PayCurrency    string    `json:"pay_currency"`
		PayAmount      string    `json:"pay_amount"`
		PaymentAddress string    `json:"payment_address"`
	}

	ListOrdersResponse struct {
		CurrentPage int64           `json:"current_page"`
		PerPage     int64           `json:"per_page"`
		TotalOrders int64           `json:"total_orders"`
		TotalPages  int64           `json:"total_pages"`
		Orders      []OrderResponse `json:"orders"`
	}

	PaymentCallbackResponse struct {
		OrderBase
		CreatedAt   string `json:"created_at" form:"created_at"`
		PayCurrency string `json:"pay_currency" form:"pay_currency"`
		PayAmount   string `json:"pay_amount" form:"pay_amount"`
		Token       string `json:"token" form:"token"`
	}
)

func (t *Template) Render(w io.Writer, name string, data interface{},
	c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func main() {
	t := &Template{
		templates: template.Must(template.ParseGlob("public/views/*.html")),
	}

	e := echo.New()
	e.Renderer = t
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	_, err := ping()
	if err != nil {
		log.Fatal(err)
	}

	e.GET("/", index)
	e.GET("/success", index)

	e.POST("/payments/paypal/webhook", index)
	e.POST("/payments/paypal/checkout/create", index)
	e.POST("/payments/paypal/checkout/execute", index)
	e.POST("/payments/coingate/callback", coingateCallback)

	e.Logger.Fatal(e.Start("127.0.0.1:1313"))
}

func index(c echo.Context) error {
	dump, err := httputil.DumpRequest(c.Request(), true)
	if err != nil {
		log.Fatal(err)
	}

	log.Print(string(dump))

	return c.Render(http.StatusOK, "payment", "World")
}

func coingateCallback(c echo.Context) error {
	p := new(PaymentCallbackResponse)

	if err := c.Bind(p); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"success": false,
			"message": err,
		})
	}

	var status string
	if p.Token == "5d02161be9bfb6192a33" {
		status = p.Status
	}

	return c.JSON(http.StatusOK, echo.Map{
		"success": true,
		"status":  status,
	})
}

func createOrder(orderID string, amount float64, priceCurrency, receiveCurrency,
	title, description, callbackURL, cancelURL, successURL, token string) (CreateOrderResponse, error) {

	path := fmt.Sprintf("%s/orders", CoinGateSandboxURL)
	log.Print(path)

	a := strconv.FormatFloat(amount, 'f', 2, 64)
	construct := url.Values{}
	construct.Add("order_id", orderID)
	construct.Add("price_amount", a)
	construct.Add("price_currency", priceCurrency)
	construct.Add("receive_currency", receiveCurrency)
	construct.Add("title", title)
	construct.Add("description", description)
	construct.Add("callback_url", callbackURL)
	construct.Add("cancel_url", cancelURL)
	construct.Add("success_url", successURL)
	construct.Add("token", token)
	message := construct.Encode()

	auth := fmt.Sprintf("Token %s", CoingateToken)

	headers := map[string]string{
		"Authorization":  auth,
		"Content-Type":   "application/x-www-form-urlencoded",
		"Content-Length": strconv.Itoa(len(message)),
	}

	data := CreateOrderResponse{}

	err := sendPayload("POST", path, headers, strings.NewReader(message), &data)
	if err != nil {
		return data, err
	}

	return data, nil
}

func getOrder(id int64) (OrderResponse, error) {
	d := strconv.FormatInt(id, 10)
	path := fmt.Sprintf("%s/orders/%s", CoinGateSandboxURL, d)
	log.Print(path)

	auth := fmt.Sprintf("Token %s", CoingateToken)

	headers := map[string]string{
		"Authorization": auth,
	}

	data := OrderResponse{}

	err := sendPayload("GET", path, headers, nil, &data)
	if err != nil {
		return data, err
	}

	return data, nil
}

func listOrders(perPage int64, page int64, sort string) (ListOrdersResponse, error) {
	a := strconv.FormatInt(perPage, 10)
	p := strconv.FormatInt(page, 10)

	construct := url.Values{}
	construct.Add("per_page", a)
	construct.Add("page", p)
	construct.Add("sort", sort)

	path := fmt.Sprintf("%s/orders?%s", CoinGateSandboxURL, construct.Encode())
	log.Print(path)

	auth := fmt.Sprintf("Token %s", CoingateToken)

	headers := map[string]string{
		"Authorization": auth,
	}

	data := ListOrdersResponse{}

	err := sendPayload("GET", path, headers, nil, &data)
	if err != nil {
		return data, err
	}

	return data, nil
}

func getExchangeRates(from, to string) (float64, error) {
	path := fmt.Sprintf("%s/rates/merchant/%s/%s", CoinGateSandboxURL, from, to)
	log.Print(path)

	var data float64
	err := sendPayload("GET", path, nil, nil, &data)
	if err != nil {
		return 0, err
	}

	return data, nil
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

func createPaymentPaypal() error {
	d := strconv.FormatInt(id, 10)
	path := fmt.Sprintf("%s/v1/payments/payment", PaypalSandboxURL)
	log.Print(path)

	auth := fmt.Sprintf("Bearer %s", PaypalSecret)

	headers := map[string]string{
		"Authorization": auth,
		"Content-Type": "application/json",
	}

	message := echo.Map{
		"intent": "sale",
		"experience_profile_id": "",
		"redirect_urls": {
			"return_url": "https://staging42.serveo.net/",
			"cancel_url": "https://staging42.serveo.net/"
		},
		"payer": {
			"payment_method": "paypal"
		},
		"transactions": []echo.Map{
			"amount": {
				"total": "4.00",
				"currency": "USD",
				"details": {
					"subtotal": "2.00",
					"shipping": "1.00",
					"tax": "2.00",
					"shipping_discount": "-1.00"
				}
			}
		},
		"note_to_payer": "Contact us for any questions on your order.",
		"description": "The payment transaction description.",
		"invoice_number": "merchant invoice",
		"custom": "merchant custom data"
	}

	err := sendPayload("GET", path, headers, nil, &data)
	if err != nil {
		return data, err
	}

	return data, nil
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
