package main

import (
	"bytes"
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

	PaypalAccessTokenResponse struct {
		Scope       string `json:"scope"`
		Nonce       string `json:"nonce"`
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		AppID       string `json:"app_id"`
		ExpiresIn   int64  `json:"expires_in"`
	}

	PaypalPaymentResponse struct {
		ID           string     `json:"id"`
		CreateTime   time.Time  `json:"create_time"`
		UpdateTime   time.Time  `json:"update_time"`
		State        string     `json:"state"`
		Intent       string     `json:"intent"`
		Payer        echo.Map   `json:"payer"`
		Transactions []echo.Map `json:"transactions"`
		Links        []echo.Map `json:"links"`
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
	e.GET("/success", success)

	e.POST("/payments/paypal/webhook", paypalNotificationWebhook)
	e.POST("/payments/paypal/checkout/create", paypalCreatePayment)
	e.POST("/payments/paypal/checkout/execute", paypalExecutePayment)
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

func success(c echo.Context) error {
	return nil
}

func paypalNotificationWebhook(c echo.Context) error {
	return nil
}

func paypalCreatePayment(c echo.Context) error {
	data, err := createPaymentPaypal()
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"success": false,
			"message": err,
		})
	}
	
	return c.JSON(http.StatusOK, echo.Map{
		"paymentID": data.ID,
	})
}

func paypalExecutePayment(c echo.Context) error {
	paymentID := c.FormValue("paymentID")
	payerID := c.FormValue("payerID")

	data, err := executePaymentPaypal(paymentID, payerID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"success": false,
			"message": err,
		})
	}

	return c.JSON(http.StatusOK, echo.Map{
		"success": true,
	})
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

func getPaypalAccessToken() (PaypalAccessTokenResponse, error) {
	path := fmt.Sprintf("%s/v1/oauth2/token", PaypalSandboxURL)
	log.Print(path)

	headers := map[string]string{
		"Accept":          "application/json",
		"Accept-Language": "en_US",
	}

	construct := url.Values{}
	construct.Add("grant_type", "client_credentials")
	message := construct.Encode()

	data := PaypalAccessTokenResponse{}

	err := sendPayloadAuth("POST", path, PaypalClientID, PaypalSecret, headers,
		strings.NewReader(message), &data)
	if err != nil {
		return data, err
	}

	return data, nil
}

func createPaymentPaypal() (PaypalPaymentResponse, error) {
	path := fmt.Sprintf("%s/v1/payments/payment", PaypalSandboxURL)
	log.Print(path)

	auth := fmt.Sprintf("Bearer %s", "A21AAELhBtesT-8vN5W67ffHUFal_cP-hY1nQf3YowxscIvaUHYPXe-H5rAsj2yn-G-CFRfmukk6GLRlJI0XI3qQe_AbEeX7w")

	headers := map[string]string{
		"Authorization": auth,
		"Content-Type":  "application/json",
	}

	message := echo.Map{
		"intent": "sale",
		"redirect_urls": echo.Map{
			"return_url": "https://staging42.serveo.net/",
			"cancel_url": "https://staging42.serveo.net/",
		},
		"payer": echo.Map{
			"payment_method": "paypal",
		},
		"transactions": []echo.Map{
			echo.Map{
				"amount": echo.Map{
					"total":    "4.00",
					"currency": "USD",
				},
			},
		},
	}

	data := PaypalPaymentResponse{}

	b, err := json.Marshal(message)
	if err != nil {
		return data, err
	}

	err = sendPayload("POST", path, headers, bytes.NewReader(b), &data)
	if err != nil {
		return data, err
	}

	return data, nil
}

func executePaymentPaypal(paymentID, payerID string) (PaypalPaymentResponse, error) {
	path := fmt.Sprintf("%s/v1/payments/payment/%s/execute/", PaypalSandboxURL, paymentID)
	log.Print(path)

	auth := fmt.Sprintf("Bearer %s", "A21AAELhBtesT-8vN5W67ffHUFal_cP-hY1nQf3YowxscIvaUHYPXe-H5rAsj2yn-G-CFRfmukk6GLRlJI0XI3qQe_AbEeX7w")

	headers := map[string]string{
		"Authorization": auth,
		"Content-Type":  "application/json",
	}

	message := echo.Map{
		"payer_id": payerID,
	}

	data := PaypalPaymentResponse{}

	b, err := json.Marshal(message)
	if err != nil {
		return data, err
	}

	err = sendPayload("POST", path, headers, bytes.NewReader(b), &data)
	if err != nil {
		return data, err
	}

	return data, nil
}

func showPaymentPaypal(paymentID string) (PaypalPaymentResponse, error) {
	path := fmt.Sprintf("%s/v1/payments/payment/%s", PaypalSandboxURL, paymentID)
	log.Print(path)

	auth := fmt.Sprintf("Bearer %s", "A21AAELhBtesT-8vN5W67ffHUFal_cP-hY1nQf3YowxscIvaUHYPXe-H5rAsj2yn-G-CFRfmukk6GLRlJI0XI3qQe_AbEeX7w")

	headers := map[string]string{
		"Authorization": auth,
		"Content-Type":  "application/json",
	}

	data := PaypalPaymentResponse{}

	err = sendPayload("POST", path, headers, nil, &data)
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

func sendPayloadAuth(method, path, username, password string,
	headers map[string]string, body io.Reader, result interface{}) error {
	method = strings.ToUpper(method)

	req, err := http.NewRequest(method, path, body)
	req.SetBasicAuth(username, password)
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
