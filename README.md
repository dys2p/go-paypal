# A PayPal API library written in Go

[![Go Reference](https://pkg.go.dev/badge/github.com/dys2p/go-paypal.svg)](https://pkg.go.dev/github.com/dys2p/go-paypal)

This library sets up a PayPal transaction and captures the transaction funds.

- https://developer.paypal.com/docs/checkout/standard/
- CSP: https://developer.paypal.com/sdk/js/configuration/

`ShippingPreference` defaults to `NO_SHIPPING`, so the PayPal window won't show the shipping information. Our example code does not send any shipping information to PayPal.

## Example of usage

For the client side, follow:

* [Set up a Transaction](https://developer.paypal.com/docs/checkout/reference/server-integration/set-up-transaction/)
* [Capture Transaction Funds](https://developer.paypal.com/docs/checkout/reference/server-integration/capture-transaction/)

On the server, first create a `paypal.Config` or load it from a JSON file:

```
paypalConfig, err := paypal.Load("paypal.json")
if err != nil {
	return err
}
```

Then set up a transaction:

```
// authenticate
authResult, err := paypalConfig.Auth()
if err != nil {
	return err
}

// call PayPal API to generate an order
generateOrderResponse, err := paypalConfig.CreateOrder(authResult, amountInCents)
if err != nil {
	return err
}

// return ID of generated order to the client
successResponse, err := json.Marshal(&paypal.SuccessResponse{OrderID: generateOrderResponse.ID})
if err != nil {
	return err
}
w.Header().Set("Content-Type", "application/json")
w.Write(successResponse)
```

Capture the funds:

```
// get the PayPal OrderID from the client, e.g. from the request body
captureRequest := &paypal.CaptureRequest{}
if err := json.NewDecoder(r.Body).Decode(captureRequest); err != nil {
	return err
}

// authenticate
authResult, err := paypalConfig.Auth()
if err != nil {
	return err
}

// call PayPal to capture the funds
captureResponse, err := paypalConfig.Capture(authResult, captureRequest.OrderID)
if err != nil {
	return err
}

// save the capture ID to your database
// captureResponse.PurchaseUnits[0].Payments.Captures[0].ID

// looks like the handler must return some JSON to the client, although the PayPal docs don't mention that
w.Header().Set("Content-Type", "application/json")
w.Write([]byte("true"))
```
