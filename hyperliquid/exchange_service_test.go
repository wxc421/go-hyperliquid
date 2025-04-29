package hyperliquid

import (
	"testing"
)

func TestOrderFlow(t *testing.T) {
	// 初始化 API
	api := NewExchangeAPI(true) // true 表示主网
	api.SetPrivateKey("YOUR_PRIVATE_KEY")
	api.SetAccountAddress("YOUR_ADDRESS")

	// 1. 准备订单请求
	requests := []OrderRequest{
		{
			Coin:    "SOL",
			IsBuy:   true,
			Sz:      0.1,
			LimitPx: 100,
			OrderType: OrderType{
				Limit: &LimitOrderType{
					Tif: "Gtc",
				},
			},
			ReduceOnly: false,
		},
	}

	// 2. 创建未签名交易
	unsignedRequest, err := api.CreateUnsignedOrder(requests, "na", false)
	if err != nil {
		t.Fatalf("创建未签名交易失败: %v", err)
	}

	// 3. 签名交易
	signedRequest, err := api.SignOrder(unsignedRequest)
	if err != nil {
		t.Fatalf("签名交易失败: %v", err)
	}

	// 4. 发送已签名交易
	response, err := api.SendSignedOrder(signedRequest)
	if err != nil {
		t.Fatalf("发送交易失败: %v", err)
	}

	t.Logf("订单响应: %+v", response)
}

func TestOrderFlowWithModification(t *testing.T) {
	// 初始化 API
	api := NewExchangeAPI(true)
	api.SetPrivateKey("YOUR_PRIVATE_KEY")
	api.SetAccountAddress("YOUR_ADDRESS")

	// 1. 准备订单请求
	requests := []OrderRequest{
		{
			Coin:    "BTC",
			IsBuy:   true,
			Sz:      0.01,
			LimitPx: 50000,
			OrderType: OrderType{
				Limit: &LimitOrderType{
					Tif: "Gtc",
				},
			},
			ReduceOnly: false,
		},
	}

	// 2. 创建未签名交易
	unsignedRequest, err := api.CreateUnsignedOrder(requests, "na", false)
	if err != nil {
		t.Fatalf("创建未签名交易失败: %v", err)
	}

	// 3. 在签名前修改订单价格
	// 这里展示了如何在签名前修改订单参数
	orderAction := unsignedRequest.Action.(PlaceOrderAction)
	if len(orderAction.Orders) > 0 {
		orderAction.Orders[0].LimitPx = "51000" // 修改价格
		unsignedRequest.Action = orderAction
	}

	// 4. 签名交易
	signedRequest, err := api.SignOrder(unsignedRequest)
	if err != nil {
		t.Fatalf("签名交易失败: %v", err)
	}

	// 5. 发送已签名交易
	response, err := api.SendSignedOrder(signedRequest)
	if err != nil {
		t.Fatalf("发送交易失败: %v", err)
	}

	t.Logf("订单响应: %v", response)
}

func TestOrderFlowWithErrorHandling(t *testing.T) {
	// 初始化 API
	api := NewExchangeAPI(true)
	api.SetPrivateKey("YOUR_PRIVATE_KEY")
	api.SetAccountAddress("YOUR_ADDRESS")

	// 1. 准备无效的订单请求（数量为0）
	requests := []OrderRequest{
		{
			Coin:    "ETH",
			IsBuy:   true,
			Sz:      0,
			LimitPx: 3000,
			OrderType: OrderType{
				Limit: &LimitOrderType{
					Tif: "Gtc",
				},
			},
			ReduceOnly: false,
		},
	}

	// 2. 创建未签名交易
	unsignedRequest, err := api.CreateUnsignedOrder(requests, "na", false)
	if err != nil {
		t.Fatalf("创建未签名交易失败: %v", err)
	}

	// 3. 签名交易
	signedRequest, err := api.SignOrder(unsignedRequest)
	if err != nil {
		t.Fatalf("签名交易失败: %v", err)
	}

	// 4. 发送已签名交易
	_, err = api.SendSignedOrder(signedRequest)
	if err != nil {
		// 这里我们期望会收到错误，因为订单数量为0
		t.Logf("预期中的错误: %v", err)
		return
	}

	t.Fatalf("不应该成功发送无效订单")
}

func TestCreateUnsignedMarketOrder(t *testing.T) {
	// 初始化 API
	api := NewExchangeAPI(true) // true 表示主网
	api.SetPrivateKey("YOUR_PRIVATE_KEY")
	api.SetAccountAddress("YOUR_ADDRESS")

	// 1. 测试市价单买入 BTC
	coin := "BTC"
	size := 0.01      // 买入 0.01 BTC
	slippage := 0.001 // 0.1% 滑点
	request, err := api.CreateUnsignedMarketOrder(coin, size, &slippage, false)
	if err != nil {
		t.Fatalf("创建市价单未签名请求失败: %v", err)
	}

	// 验证请求内容
	if request.Action == nil {
		t.Fatal("请求的 Action 为空")
	}

	// 验证签名
	signedRequest, err := api.SignOrder(request)
	if err != nil {
		t.Fatalf("签名失败: %v", err)
	}

	// 发送订单
	response, err := api.SendSignedOrder(signedRequest)
	if err != nil {
		t.Fatalf("发送订单失败: %v", err)
	}

	t.Logf("市价单响应: %+v", response)
}

func TestCreateUnsignedLimitOrder(t *testing.T) {
	// 初始化 API
	api := NewExchangeAPI(true)
	api.SetPrivateKey("YOUR_PRIVATE_KEY")
	api.SetAccountAddress("YOUR_ADDRESS")

	// 1. 测试限价单买入 BTC
	coin := "BTC"
	size := 0.01        // 买入 0.01 BTC
	price := 50000.0    // 限价 50000
	orderType := TifGtc // 一直有效直到取消
	reduceOnly := false // 非只减仓
	request, err := api.CreateUnsignedLimitOrder(coin, size, price, orderType, reduceOnly, false)
	if err != nil {
		t.Fatalf("创建限价单未签名请求失败: %v", err)
	}

	// 验证请求内容
	if request.Action == nil {
		t.Fatal("请求的 Action 为空")
	}

	// 验证签名
	signedRequest, err := api.SignOrder(request)
	if err != nil {
		t.Fatalf("签名失败: %v", err)
	}

	// 发送订单
	response, err := api.SendSignedOrder(signedRequest)
	if err != nil {
		t.Fatalf("发送订单失败: %v", err)
	}

	t.Logf("限价单响应: %+v", response)
}

func TestCreateUnsignedOrderErrorHandling(t *testing.T) {
	// 初始化 API
	api := NewExchangeAPI(true)
	api.SetPrivateKey("YOUR_PRIVATE_KEY")
	api.SetAccountAddress("YOUR_ADDRESS")

	// 1. 测试无效的订单类型
	_, err := api.CreateUnsignedLimitOrder("BTC", 0.01, 50000.0, "InvalidType", false, false)
	if err == nil {
		t.Fatal("应该返回无效订单类型的错误")
	}
	t.Logf("预期中的错误: %v", err)

	// 2. 测试数量为0的订单
	_, err = api.CreateUnsignedLimitOrder("BTC", 0.0, 50000.0, TifGtc, false, false)
	if err == nil {
		t.Fatal("应该返回数量为0的错误")
	}
	t.Logf("预期中的错误: %v", err)

	// 3. 测试价格为0的订单
	_, err = api.CreateUnsignedLimitOrder("BTC", 0.01, 0.0, TifGtc, false, false)
	if err == nil {
		t.Fatal("应该返回价格为0的错误")
	}
	t.Logf("预期中的错误: %v", err)
}

func TestCreateUnsignedMarketOrderSpot(t *testing.T) {
	// 初始化 API
	api := NewExchangeAPI(true)
	api.SetPrivateKey("YOUR_PRIVATE_KEY")
	api.SetAccountAddress("YOUR_ADDRESS")

	// 1. 测试现货市价单
	coin := "HYPE"
	size := 0.1       // 买入 0.1 HYPE
	slippage := 0.001 // 0.1% 滑点
	request, err := api.CreateUnsignedMarketOrder(coin, size, &slippage, true)
	if err != nil {
		t.Fatalf("创建现货市价单未签名请求失败: %v", err)
	}

	// 验证请求内容
	if request.Action == nil {
		t.Fatal("请求的 Action 为空")
	}

	// 验证签名
	signedRequest, err := api.SignOrder(request)
	if err != nil {
		t.Fatalf("签名失败: %v", err)
	}

	// 发送订单
	response, err := api.SendSignedOrder(signedRequest)
	if err != nil {
		t.Fatalf("发送订单失败: %v", err)
	}

	t.Logf("现货市价单响应: %+v", response)
}
