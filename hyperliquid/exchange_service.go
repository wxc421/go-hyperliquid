package hyperliquid

import (
	"fmt"
	"math"

	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// IExchangeAPI is an interface for the /exchange service.
type IExchangeAPI interface {
	IClient

	// Open orders
	BulkOrders(requests []OrderRequest, grouping Grouping) (*OrderResponse, error)
	Order(request OrderRequest, grouping Grouping) (*OrderResponse, error)
	MarketOrder(coin string, size float64, slippage *float64, clientOID ...string) (*OrderResponse, error)
	LimitOrder(orderType string, coin string, size float64, px float64, isBuy bool, reduceOnly bool, clientOID ...string) (*OrderResponse, error)

	// Order management
	CancelOrderByOID(coin string, orderID int) (any, error)
	CancelOrderByCloid(coin string, clientOID string) (any, error)
	BulkCancelOrders(cancels []CancelOidWire) (any, error)
	CancelAllOrdersByCoin(coin string) (any, error)
	CancelAllOrders() (any, error)
	ClosePosition(coin string) (*OrderResponse, error)

	// Account management
	Withdraw(destination string, amount float64) (*WithdrawResponse, error)
	UpdateLeverage(coin string, isCross bool, leverage int) (any, error)
}

// Implement the IExchangeAPI interface.
type ExchangeAPI struct {
	Client
	infoAPI      *InfoAPI
	address      string
	baseEndpoint string
	meta         map[string]AssetInfo
	spotMeta     map[string]AssetInfo
}

// NewExchangeAPI creates a new default ExchangeAPI.
// Run SetPrivateKey() and SetAccountAddress() to set the private key and account address.
func NewExchangeAPI(isMainnet bool) *ExchangeAPI {
	api := ExchangeAPI{
		Client:       *NewClient(isMainnet),
		baseEndpoint: "/exchange",
		infoAPI:      NewInfoAPI(isMainnet),
		address:      "",
	}
	// turn on debug mode if there is an error with /info service
	meta, err := api.infoAPI.BuildMetaMap()
	if err != nil {
		api.SetDebugActive()
		api.debug("Error building meta map: %s", err)
	}
	api.meta = meta

	spotMeta, err := api.infoAPI.BuildSpotMetaMap()
	if err != nil {
		api.SetDebugActive()
		api.debug("Error building spot meta map: %s", err)
	}
	api.spotMeta = spotMeta

	return &api
}

//
// Helpers
//

func (api *ExchangeAPI) Endpoint() string {
	return api.baseEndpoint
}

// Helper function to calculate the slippage price based on the market price.
func (api *ExchangeAPI) SlippagePrice(coin string, isBuy bool, slippage float64) float64 {
	marketPx, err := api.infoAPI.GetMartketPx(coin)
	if err != nil {
		api.debug("Error getting market price: %s", err)
		return 0.0
	}
	return CalculateSlippage(isBuy, marketPx, slippage)
}

// SlippagePriceSpot is a helper function to calculate the slippage price for a spot coin.
func (api *ExchangeAPI) SlippagePriceSpot(coin string, isBuy bool, slippage float64) float64 {
	marketPx, err := api.infoAPI.GetSpotMarketPx(coin)
	if err != nil {
		api.debug("Error getting market price: %s", err)
		return 0.0
	}
	slippagePrice := CalculateSlippage(isBuy, marketPx, slippage)
	return slippagePrice
}

// Helper function to get the chain params based on the network type.
func (api *ExchangeAPI) getChainParams() (string, string) {
	if api.IsMainnet() {
		return "0xa4b1", "Mainnet"
	}
	return "0x66eee", "Testnet"
}

// Build bulk orders EIP712 message
func (api *ExchangeAPI) BuildBulkOrdersEIP712(requests []OrderRequest, grouping Grouping) (apitypes.TypedData, error) {
	var wires []OrderWire
	for _, req := range requests {
		wires = append(wires, OrderRequestToWire(req, api.meta, false))
	}
	timestamp := GetNonce()
	action := OrderWiresToOrderAction(wires, grouping)
	srequest, err := api.BuildEIP712Message(action, timestamp)
	if err != nil {
		api.debug("Error building EIP712 message: %s", err)
		return apitypes.TypedData{}, err
	}
	return SignRequestToEIP712TypedData(srequest), nil
}

// Build order EIP712 message
func (api *ExchangeAPI) BuildOrderEIP712(request OrderRequest, grouping Grouping) (apitypes.TypedData, error) {
	return api.BuildBulkOrdersEIP712([]OrderRequest{request}, grouping)
}

//
// Base Methods
//

// Place orders in bulk
// https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/exchange-endpoint#place-an-order
func (api *ExchangeAPI) BulkOrders(requests []OrderRequest, grouping Grouping, isSpot bool) (*OrderResponse, error) {
	var wires []OrderWire
	var meta map[string]AssetInfo
	if isSpot {
		meta = api.spotMeta
	} else {
		meta = api.meta
	}
	for _, req := range requests {
		wires = append(wires, OrderRequestToWire(req, meta, isSpot))
	}
	timestamp := GetNonce()
	action := OrderWiresToOrderAction(wires, grouping)
	v, r, s, err := api.SignL1Action(action, timestamp)
	if err != nil {
		api.debug("Error signing L1 action: %s", err)
		return nil, err
	}
	request := ExchangeRequest{
		Action:       action,
		Nonce:        timestamp,
		Signature:    ToTypedSig(r, s, v),
		VaultAddress: nil,
	}
	return MakeUniversalRequest[OrderResponse](api, request)
}

// Cancel order(s)
// https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/exchange-endpoint#cancel-order-s
func (api *ExchangeAPI) BulkCancelOrders(cancels []CancelOidWire) (*OrderResponse, error) {
	timestamp := GetNonce()
	action := CancelOidOrderAction{
		Type:    "cancel",
		Cancels: cancels,
	}
	v, r, s, err := api.SignL1Action(action, timestamp)
	if err != nil {
		api.debug("Error signing L1 action: %s", err)
		return nil, err
	}
	request := ExchangeRequest{
		Action:       action,
		Nonce:        timestamp,
		Signature:    ToTypedSig(r, s, v),
		VaultAddress: nil,
	}
	return MakeUniversalRequest[OrderResponse](api, request)
}

// Bulk modify orders
// https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/exchange-endpoint#modify-multiple-orders
func (api *ExchangeAPI) BulkModifyOrders(modifyRequests []ModifyOrderRequest, isSpot bool) (*OrderResponse, error) {
	wires := []ModifyOrderWire{}

	for _, req := range modifyRequests {
		wires = append(wires, ModifyOrderRequestToWire(req, api.meta, isSpot))
	}
	action := ModifyOrderAction{
		Type:     "batchModify",
		Modifies: wires,
	}

	timestamp := GetNonce()
	vVal, rVal, sVal, signErr := api.SignL1Action(action, timestamp)
	if signErr != nil {
		return nil, signErr
	}
	request := ExchangeRequest{
		Action:       action,
		Nonce:        timestamp,
		Signature:    ToTypedSig(rVal, sVal, vVal),
		VaultAddress: nil,
	}
	return MakeUniversalRequest[OrderResponse](api, request)
}

// Cancel exact order by Client Order Id
// https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/exchange-endpoint#cancel-order-s-by-cloid
func (api *ExchangeAPI) CancelOrderByCloid(coin string, clientOID string) (*OrderResponse, error) {
	timestamp := GetNonce()
	action := CancelCloidOrderAction{
		Type: "cancelByCloid",
		Cancels: []CancelCloidWire{
			{
				Asset: api.meta[coin].AssetId,
				Cloid: clientOID,
			},
		},
	}
	v, r, s, err := api.SignL1Action(action, timestamp)
	if err != nil {
		api.debug("Error signing L1 action: %s", err)
		return nil, err
	}
	request := ExchangeRequest{
		Action:       action,
		Nonce:        timestamp,
		Signature:    ToTypedSig(r, s, v),
		VaultAddress: nil,
	}
	return MakeUniversalRequest[OrderResponse](api, request)
}

// Update leverage for a coin
// https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/exchange-endpoint#update-leverage
func (api *ExchangeAPI) UpdateLeverage(coin string, isCross bool, leverage int) (*DefaultExchangeResponse, error) {
	timestamp := GetNonce()
	action := UpdateLeverageAction{
		Type:     "updateLeverage",
		Asset:    api.meta[coin].AssetId,
		IsCross:  isCross,
		Leverage: leverage,
	}
	v, r, s, err := api.SignL1Action(action, timestamp)
	if err != nil {
		api.debug("Error signing L1 action: %s", err)
		return nil, err
	}
	request := ExchangeRequest{
		Action:       action,
		Nonce:        timestamp,
		Signature:    ToTypedSig(r, s, v),
		VaultAddress: nil,
	}
	return MakeUniversalRequest[DefaultExchangeResponse](api, request)
}

// Initiate a withdraw request
// https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/exchange-endpoint#initiate-a-withdrawal-request
func (api *ExchangeAPI) Withdraw(destination string, amount float64) (*WithdrawResponse, error) {
	nonce := GetNonce()
	action := WithdrawAction{
		Type:        "withdraw3",
		Destination: destination,
		Amount:      SizeToWire(amount, USDC_SZ_DECIMALS),
		Time:        nonce,
	}
	signatureChainID, chainType := api.getChainParams()
	action.HyperliquidChain = chainType
	action.SignatureChainID = signatureChainID
	v, r, s, err := api.SignWithdrawAction(action)
	if err != nil {
		api.debug("Error signing withdraw action: %s", err)
		return nil, err
	}
	request := &ExchangeRequest{
		Action:       action,
		Nonce:        nonce,
		Signature:    ToTypedSig(r, s, v),
		VaultAddress: nil,
	}
	return MakeUniversalRequest[WithdrawResponse](api, request)
}

//
// Connectors Methods
//

// Place single order
func (api *ExchangeAPI) Order(request OrderRequest, grouping Grouping) (*OrderResponse, error) {
	return api.BulkOrders([]OrderRequest{request}, grouping, false)
}

// Open a market order.
// Limit order with TIF=IOC and px=market price * (1 +- slippage).
// Size determines the amount of the coin to buy/sell.
//
//	MarketOrder("BTC", 0.1, nil) // Buy 0.1 BTC
//	MarketOrder("BTC", -0.1, nil) // Sell 0.1 BTC
//	MarketOrder("BTC", 0.1, &slippage) // Buy 0.1 BTC with slippage
func (api *ExchangeAPI) MarketOrder(coin string, size float64, slippage *float64, clientOID ...string) (*OrderResponse, error) {
	slpg := GetSlippage(slippage)
	isBuy := IsBuy(size)
	finalPx := api.SlippagePrice(coin, isBuy, slpg)
	orderType := OrderType{
		Limit: &LimitOrderType{
			Tif: TifIoc,
		},
	}
	orderRequest := OrderRequest{
		Coin:       coin,
		IsBuy:      isBuy,
		Sz:         math.Abs(size),
		LimitPx:    finalPx,
		OrderType:  orderType,
		ReduceOnly: false,
	}
	if len(clientOID) > 0 {
		orderRequest.Cloid = clientOID[0]
	}
	return api.Order(orderRequest, GroupingNa)
}

// MarketOrderSpot is a market order for a spot coin.
// It is used to buy/sell a spot coin.
// Limit order with TIF=IOC and px=market price * (1 +- slippage).
// Size determines the amount of the coin to buy/sell.
//
//	MarketOrderSpot("HYPE", 0.1, nil) // Buy 0.1 HYPE
//	MarketOrderSpot("HYPE", -0.1, nil) // Sell 0.1 HYPE
//	MarketOrderSpot("HYPE", 0.1, &slippage) // Buy 0.1 HYPE with slippage
func (api *ExchangeAPI) MarketOrderSpot(coin string, size float64, slippage *float64) (*OrderResponse, error) {
	slpg := GetSlippage(slippage)
	isBuy := IsBuy(size)
	finalPx := api.SlippagePriceSpot(coin, isBuy, slpg)
	orderType := OrderType{
		Limit: &LimitOrderType{
			Tif: TifIoc,
		},
	}
	orderRequest := OrderRequest{
		Coin:       coin,
		IsBuy:      isBuy,
		Sz:         math.Abs(size),
		LimitPx:    finalPx,
		OrderType:  orderType,
		ReduceOnly: false,
	}
	return api.OrderSpot(orderRequest, GroupingNa)
}

// Open a limit order.
// Order type can be Gtc, Ioc, Alo.
// Size determines the amount of the coin to buy/sell.
// See the constants TifGtc, TifIoc, TifAlo.
func (api *ExchangeAPI) LimitOrder(orderType string, coin string, size float64, px float64, reduceOnly bool, clientOID ...string) (*OrderResponse, error) {
	// check if the order type is valid
	if orderType != TifGtc && orderType != TifIoc && orderType != TifAlo {
		return nil, APIError{Message: fmt.Sprintf("Invalid order type: %s. Available types: %s, %s, %s", orderType, TifGtc, TifIoc, TifAlo)}
	}
	orderTypeZ := OrderType{
		Limit: &LimitOrderType{
			Tif: orderType,
		},
	}
	orderRequest := OrderRequest{
		Coin:       coin,
		IsBuy:      IsBuy(size),
		Sz:         math.Abs(size),
		LimitPx:    px,
		OrderType:  orderTypeZ,
		ReduceOnly: reduceOnly,
	}
	if len(clientOID) > 0 {
		orderRequest.Cloid = clientOID[0]
	}
	return api.Order(orderRequest, GroupingNa)
}

// Close all positions for a given coin. They are closing with a market order.
func (api *ExchangeAPI) ClosePosition(coin string) (*OrderResponse, error) {
	// Get all positions and find the one for the coin
	// Then just make MarketOpen with the reverse size
	state, err := api.infoAPI.GetUserState(api.AccountAddress())
	if err != nil {
		api.debug("Error GetUserState: %s", err)
		return nil, err
	}
	positions := state.AssetPositions
	slippage := GetSlippage(nil)

	// Find the position for the coin
	for _, position := range positions {
		item := position.Position
		if coin != item.Coin {
			continue
		}
		size := item.Szi
		// reverse the position to close
		isBuy := !IsBuy(size)
		finalPx := api.SlippagePrice(coin, isBuy, slippage)
		orderType := OrderType{
			Limit: &LimitOrderType{
				Tif: "Ioc",
			},
		}
		orderRequest := OrderRequest{
			Coin:       coin,
			IsBuy:      isBuy,
			Sz:         math.Abs(size),
			LimitPx:    finalPx,
			OrderType:  orderType,
			ReduceOnly: true,
		}
		return api.Order(orderRequest, GroupingNa)
	}
	return nil, APIError{Message: fmt.Sprintf("No position found for %s", coin)}
}

// OrderSpot places a spot order
func (api *ExchangeAPI) OrderSpot(request OrderRequest, grouping Grouping) (*OrderResponse, error) {
	return api.BulkOrders([]OrderRequest{request}, grouping, true)
}

// Cancel exact order by OID
func (api *ExchangeAPI) CancelOrderByOID(coin string, orderID int64) (*OrderResponse, error) {
	return api.BulkCancelOrders([]CancelOidWire{{Asset: api.meta[coin].AssetId, Oid: int(orderID)}})
}

// Cancel all orders for a given coin
func (api *ExchangeAPI) CancelAllOrdersByCoin(coin string) (*OrderResponse, error) {
	orders, err := api.infoAPI.GetOpenOrders(api.AccountAddress())
	if err != nil {
		api.debug("Error getting orders: %s", err)
		return nil, err
	}
	var cancels []CancelOidWire
	for _, order := range *orders {
		if coin != order.Coin {
			continue
		}
		cancels = append(cancels, CancelOidWire{Asset: api.meta[coin].AssetId, Oid: int(order.Oid)})
	}
	return api.BulkCancelOrders(cancels)
}

// Cancel all open orders
func (api *ExchangeAPI) CancelAllOrders() (*OrderResponse, error) {
	orders, err := api.infoAPI.GetOpenOrders(api.AccountAddress())
	if err != nil {
		api.debug("Error getting orders: %s", err)
		return nil, err
	}
	if len(*orders) == 0 {
		return nil, APIError{Message: "No open orders to cancel"}
	}
	var cancels []CancelOidWire
	for _, order := range *orders {
		cancels = append(cancels, CancelOidWire{Asset: api.meta[order.Coin].AssetId, Oid: int(order.Oid)})
	}
	return api.BulkCancelOrders(cancels)
}

// CreateUnsignedOrder creates an unsigned order request
// Similar to MarketOrder and LimitOrder, but returns the unsigned request instead of sending it
func (api *ExchangeAPI) CreateUnsignedOrder(coin string, size float64, price float64, orderType string, reduceOnly bool, isSpot bool) (*ExchangeRequest, error) {
	// 构建订单类型
	var orderTypeObj OrderType
	if orderType == TifGtc || orderType == TifIoc || orderType == TifAlo {
		orderTypeObj = OrderType{
			Limit: &LimitOrderType{
				Tif: orderType,
			},
		}
	} else {
		return nil, APIError{Message: fmt.Sprintf("Invalid order type: %s. Available types: %s, %s, %s", orderType, TifGtc, TifIoc, TifAlo)}
	}

	// 构建订单请求
	request := OrderRequest{
		Coin:       coin,
		IsBuy:      IsBuy(size),
		Sz:         math.Abs(size),
		LimitPx:    price,
		OrderType:  orderTypeObj,
		ReduceOnly: reduceOnly,
	}

	// 转换为订单线
	var wires []OrderWire
	var meta map[string]AssetInfo
	if isSpot {
		meta = api.spotMeta
	} else {
		meta = api.meta
	}
	wires = append(wires, OrderRequestToWire(request, meta, isSpot))

	// 创建订单动作
	timestamp := GetNonce()
	action := OrderWiresToOrderAction(wires, GroupingNa)

	return &ExchangeRequest{
		Action:       action,
		Nonce:        timestamp,
		VaultAddress: nil,
	}, nil
}

func (api *ExchangeAPI) SignOrder(unsignedRequest *ExchangeRequest) (*ExchangeRequest, error) {
	v, r, s, err := api.SignL1Action(unsignedRequest.Action, unsignedRequest.Nonce)
	if err != nil {
		api.debug("Error signing L1 action: %s", err)
		return nil, err
	}

	signedRequest := *unsignedRequest
	signedRequest.Signature = ToTypedSig(r, s, v)
	return &signedRequest, nil
}

func (api *ExchangeAPI) SendSignedOrder(signedRequest *ExchangeRequest) (*OrderResponse, error) {
	return MakeUniversalRequest[OrderResponse](api, *signedRequest)
}

// CreateUnsignedMarketOrder creates an unsigned market order request
func (api *ExchangeAPI) CreateUnsignedMarketOrder(coin string, size float64, slippage *float64, isSpot bool) (*ExchangeRequest, error) {
	slpg := GetSlippage(slippage)
	isBuy := IsBuy(size)
	var finalPx float64
	if isSpot {
		finalPx = api.SlippagePriceSpot(coin, isBuy, slpg)
	} else {
		finalPx = api.SlippagePrice(coin, isBuy, slpg)
	}

	// 构建订单类型
	orderTypeObj := OrderType{
		Limit: &LimitOrderType{
			Tif: TifIoc,
		},
	}

	// 构建订单请求
	request := OrderRequest{
		Coin:       coin,
		IsBuy:      isBuy,
		Sz:         math.Abs(size),
		LimitPx:    finalPx,
		OrderType:  orderTypeObj,
		ReduceOnly: false,
	}

	// 转换为订单线
	var wires []OrderWire
	var meta map[string]AssetInfo
	if isSpot {
		meta = api.spotMeta
	} else {
		meta = api.meta
	}
	wires = append(wires, OrderRequestToWire(request, meta, isSpot))

	// 创建订单动作
	timestamp := GetNonce()
	action := OrderWiresToOrderAction(wires, GroupingNa)

	return &ExchangeRequest{
		Action:       action,
		Nonce:        timestamp,
		VaultAddress: nil,
	}, nil
}

// CreateUnsignedLimitOrder creates an unsigned limit order request
func (api *ExchangeAPI) CreateUnsignedLimitOrder(coin string, size float64, price float64, orderType string, reduceOnly bool, isSpot bool) (*ExchangeRequest, error) {
	// 检查订单类型是否有效
	if orderType != TifGtc && orderType != TifIoc && orderType != TifAlo {
		return nil, APIError{Message: fmt.Sprintf("Invalid order type: %s. Available types: %s, %s, %s", orderType, TifGtc, TifIoc, TifAlo)}
	}

	// 构建订单类型
	orderTypeObj := OrderType{
		Limit: &LimitOrderType{
			Tif: orderType,
		},
	}

	// 构建订单请求
	request := OrderRequest{
		Coin:       coin,
		IsBuy:      IsBuy(size),
		Sz:         math.Abs(size),
		LimitPx:    price,
		OrderType:  orderTypeObj,
		ReduceOnly: reduceOnly,
	}

	// 转换为订单线
	var wires []OrderWire
	var meta map[string]AssetInfo
	if isSpot {
		meta = api.spotMeta
	} else {
		meta = api.meta
	}
	wires = append(wires, OrderRequestToWire(request, meta, isSpot))

	// 创建订单动作
	timestamp := GetNonce()
	action := OrderWiresToOrderAction(wires, GroupingNa)

	return &ExchangeRequest{
		Action:       action,
		Nonce:        timestamp,
		VaultAddress: nil,
	}, nil
}
