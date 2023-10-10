package paypal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Config struct {
	OAuthAPI string `json:"oauth-api"`
	OrderAPI string `json:"order-api"`
	ClientID string `json:"client-id"`
	Secret   string `json:"secret"`
}

// Load unmarshals a json config file into a Config.
// If the file doesn't exist, it is created and an error is returned.
func Load(jsonPath string) (*Config, error) {
	data, err := os.ReadFile(jsonPath)
	if os.IsNotExist(err) {
		return nil, Create(jsonPath)
	}
	if err != nil {
		return nil, err
	}

	var config = &Config{}
	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}

	if config.OAuthAPI == "" {
		return nil, errors.New("missing oauth-api in paypal config file")
	}
	if config.OrderAPI == "" {
		return nil, errors.New("missing order-api in paypal config file")
	}
	if config.ClientID == "" {
		return nil, errors.New("missing client-id in paypal config file")
	}
	if config.Secret == "" {
		return nil, errors.New("missing secret in paypal config file")
	}
	return config, nil
}

// Create creates an empty json config file with empty values and chmod 600, so someone can fill in easily.
// Create always returns an error.
func Create(jsonPath string) error {
	data, err := json.Marshal(&Config{})
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, data, 0600); err != nil {
		return err
	}
	return fmt.Errorf("created empty config file: %s", jsonPath)
}

type AuthResult struct {
	Scope       string `json:"scope"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	AppID       string `json:"app_id"`
	ExpiresIn   int    `json:"expires_in"`
	Nonce       string `json:"nonce"`
}

type OrderRequest struct {
	Intent             string             `json:"intent"`
	PurchaseUnits      []PurchaseUnit     `json:"purchase_units"`
	ApplicationContext ApplicationContext `json:"application_context"`
}

// See https://developer.paypal.com/docs/api/orders/v2/#definition-purchase_unit
type PurchaseUnit struct {
	Amount      Amount `json:"amount"`
	Description string `json:"description"` // "[ 1 .. 127 ] characters: The purchase description."
	CustomID    string `json:"custom_id"`   // "[ 1 .. 127 ] characters: The API caller-provided external ID. Used to reconcile API caller-initiated transactions with PayPal transactions. Appears in transaction and settlement reports."
	InvoiceID   string `json:"invoice_id"`  // "[ 1 .. 127 ] characters: The API caller-provided external invoice ID for this order. Appears in both the payer's transaction history and the emails that the payer receives."
}

type Amount struct {
	CurrencyCode string  `json:"currency_code"`
	Value        float64 `json:"value"`
}

type ApplicationContext struct {
	ShippingPreference string `json:"shipping_preference"`
}

type GenerateOrderResponse struct {
	ID     string `json:"id"`     // like "1AB23456CD789012E"
	Status string `json:"status"` // like "CREATED"
	Links  []struct {
		Href   string `json:"href"`
		Rel    string `json:"rel"`
		Method string `json:"method"`
	} `json:"links"`
}

type SuccessResponse struct {
	OrderID string `json:"id"`
}

// Auth gets an access token from the PayPal API.
func (config *Config) Auth() (*AuthResult, error) {

	req, err := http.NewRequest(
		http.MethodPost,
		config.OAuthAPI,
		bytes.NewBuffer([]byte("grant_type=client_credentials")),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Accept", "application/json")
	req.SetBasicAuth(config.ClientID, config.Secret)

	resp, err := (&http.Client{
		Timeout: 10 * time.Second,
	}).Do(req)
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error getting auth: %s: %s", resp.Status, body)
	}

	var authResult = &AuthResult{}
	return authResult, json.Unmarshal(body, authResult)
}

// CreateOrder calls PayPal to set up a transaction.
func (config *Config) CreateOrder(auth *AuthResult, description, customID, invoiceID string, euroCents int) (*GenerateOrderResponse, error) {

	orderRequest := &OrderRequest{
		Intent: "CAPTURE",
		PurchaseUnits: []PurchaseUnit{
			PurchaseUnit{
				Amount: Amount{
					CurrencyCode: "EUR",
					Value:        float64(euroCents) / 100.0,
				},
				Description: description,
				CustomID:    customID,
				InvoiceID:   invoiceID,
			},
		},
		ApplicationContext: ApplicationContext{
			ShippingPreference: "NO_SHIPPING",
		},
	}

	orJson, err := json.Marshal(orderRequest)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		http.MethodPost,
		config.OrderAPI,
		bytes.NewBuffer(orJson),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Add("Content-Type", "application/json")

	resp, err := (&http.Client{
		Timeout: 10 * time.Second,
	}).Do(req)
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("error doing order: %s: %s", resp.Status, body)
	}

	generateOrderResponse := &GenerateOrderResponse{}
	return generateOrderResponse, json.Unmarshal(body, generateOrderResponse)
}

type CaptureRequest struct {
	OrderID string `json:"orderID"`
}

type CaptureResponse struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	PurchaseUnits []struct {
		ReferenceID string `json:"reference_id"`
		Shipping    struct {
			Name struct {
				FullName string `json:"full_name"`
			} `json:"name"`
			Address struct {
				AddressLine1 string `json:"address_line_1"`
				AdminArea2   string `json:"admin_area_2"`
				AdminArea1   string `json:"admin_area_1"`
				PostalCode   string `json:"postal_code"`
				CountryCode  string `json:"country_code"`
			} `json:"address"`
		} `json:"shipping"`
		Payments struct {
			Captures []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Amount struct {
					CurrencyCode string `json:"currency_code"`
					Value        string `json:"value"`
				} `json:"amount"`
				FinalCapture     bool `json:"final_capture"`
				SellerProtection struct {
					Status            string   `json:"status"`
					DisputeCategories []string `json:"dispute_categories"`
				} `json:"seller_protection"`
				SellerReceivableBreakdown struct {
					GrossAmount struct {
						CurrencyCode string `json:"currency_code"`
						Value        string `json:"value"`
					} `json:"gross_amount"`
					PaypalFee struct {
						CurrencyCode string `json:"currency_code"`
						Value        string `json:"value"`
					} `json:"paypal_fee"`
					NetAmount struct {
						CurrencyCode string `json:"currency_code"`
						Value        string `json:"value"`
					} `json:"net_amount"`
				} `json:"seller_receivable_breakdown"`
				Links []struct {
					Href   string `json:"href"`
					Rel    string `json:"rel"`
					Method string `json:"method"`
				} `json:"links"`
				CreateTime time.Time `json:"create_time"`
				UpdateTime time.Time `json:"update_time"`
			} `json:"captures"`
		} `json:"payments"`
	} `json:"purchase_units"`
	Payer struct {
		Name struct {
			GivenName string `json:"given_name"`
			Surname   string `json:"surname"`
		} `json:"name"`
		EmailAddress string `json:"email_address"`
		PayerID      string `json:"payer_id"`
		Address      struct {
			CountryCode string `json:"country_code"`
		} `json:"address"`
	} `json:"payer"`
	Links []struct {
		Href   string `json:"href"`
		Rel    string `json:"rel"`
		Method string `json:"method"`
	} `json:"links"`
}

// Capture calls PayPal to capture the order.
func (config *Config) Capture(auth *AuthResult, orderID string) (*CaptureResponse, error) {

	req, err := http.NewRequest(
		http.MethodPost,
		ensureTrailingSlash(config.OrderAPI)+orderID+"/capture",
		nil,
	)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Add("Content-Type", "application/json")

	resp, err := (&http.Client{
		Timeout: 10 * time.Second,
	}).Do(req)
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("error capturing: %s: %s", resp.Status, body)
	}

	captureResponse := &CaptureResponse{}
	return captureResponse, json.Unmarshal(body, captureResponse)
}

func ensureTrailingSlash(s string) string {
	if strings.HasSuffix(s, "/") {
		return s
	} else {
		return s + "/"
	}
}
