
# GRPC 四种传输模式

## 项目
- 项目目录： dev/grpc/basic
- 项目文件：
```shell
├── client
│   └── main.go
├── go.mod
├── go.sum
├── order
│   ├── order.pb.go
│   ├── order.proto
│   └── order_grpc.pb.go
└── server
    ├── main.go
    └── order.go

```
- Proto

```shell
syntax = "proto3";

package order;


option go_package = "order/";
option java_multiple_files = true;

import "google/protobuf/wrappers.proto";
import "google/protobuf/timestamp.proto";

message Order {
  string id = 1;
  repeated string items = 2;
  string description = 3;
  float price = 4;
  string destination = 5;

  enum AddressType {
    HOME = 0;
    WORK = 1;
  }

  message Address {
    string address = 1;
    AddressType type = 2;
  }

  repeated Address addresses = 6;

  google.protobuf.Timestamp last_updated = 7;

}

message CombinedShipment {
  string id = 1;
  string status = 2;
  repeated Order orderList = 3;
}

service OrderManagement {
  rpc addOrder(Order) returns (google.protobuf.StringValue);
  rpc getOrder(google.protobuf.StringValue) returns (Order);
  rpc searchOrders(google.protobuf.StringValue) returns (stream Order);
  rpc updateOrders(stream Order) returns (google.protobuf.StringValue);
  rpc processOrders(stream google.protobuf.StringValue)
      returns (stream CombinedShipment);
}

```

- 生成代码

```shell
 protoc -I order/  --go_out=. --go-grpc_out=. order.proto
```

- 服务端数据

```go
var (
	_      pb.OrderManagementServer = &OrderManagementImpl{}
	orders                          = make(map[string]pb.Order, 0)
)

```

## 单向调用

- 服务端实现

```go

// AddOrder Simple RPC
func (s *OrderManagementImpl) AddOrder(ctx context.Context, orderReq *pb.Order) (*wrapperspb.StringValue, error) {
	log.Println("AddOrder:")
	log.Printf("Order Added. ID : %v", orderReq.Id)
	orders[orderReq.Id] = *orderReq
	return &wrapperspb.StringValue{Value: "Order Added: " + orderReq.Id}, nil
}

// GetOrder Simple RPC
func (s *OrderManagementImpl) GetOrder(ctx context.Context, orderId *wrapperspb.StringValue) (*pb.Order, error) {
	log.Println("GetOrder:")
	ord, exists := orders[orderId.Value]
	if exists {
		return &ord, status.New(codes.OK, "").Err()
	}

	return nil, status.Errorf(codes.NotFound, "Order does not exist. : ", orderId)
}
```

- 客户端

```go

func UnaryRPC(c pb.OrderManagementClient, ctx context.Context) {
	// Add Order
	order := pb.Order{Id: "101", Items: []string{"iPhone XS", "Mac Book Pro"}, Destination: "San Jose, CA", Price: 2300.00}
	res, err := c.AddOrder(ctx, &order)
	if err != nil {
		panic(err)
	}

	log.Print("AddOrder Response -> ", res.Value)

	// Get Order
	retrievedOrder, err := c.GetOrder(ctx, &wrapperspb.StringValue{Value: "101"})
	if err != nil {
		panic(err)
	}

	log.Print("GetOrder Response -> : ", retrievedOrder)

	// Add Order
	order = pb.Order{Id: "102", Items: []string{"Oppo", "Vivo"}, Destination: "Tom, Shanghai", Price: 1300.00}
	res, err = c.AddOrder(ctx, &order)
	if err != nil {
		panic(err)
	}

	log.Print("AddOrder Response -> ", res.Value)

	// Get Order
	retrievedOrder, err = c.GetOrder(ctx, &wrapperspb.StringValue{Value: "102"})
	if err != nil {
		panic(err)
	}

	log.Print("GetOrder Response -> : ", retrievedOrder)

}

```

- tproxy

```shell
 tproxy -p 9008 -r localhost:9009 -t grpc
```


- 可以观察GRPC调用协议情况

1. 调用 AddOrder 和 GetOrder 分别用stream:1 和 stream:3

```shell
(base) ➜  GolandProjects tproxy -p 9008 -r localhost:9009 -t grpc
20:05:31.766 Listening on 127.0.0.1:9008...
20:05:47.199 [1] Accepted from: 127.0.0.1:52784
20:05:47.202 [1] Connected to server: 127.0.0.1:9009

20:05:47.203 from CLIENT [1]
grpc:(http2:preface)
00000000  50 52 49 20 2a 20 48 54  54 50 2f 32 2e 30 0d 0a  |PRI * HTTP/2.0..|
00000010  0d 0a 53 4d 0d 0a 0d 0a                           |..SM....|

20:05:47.203 from SERVER [1]
grpc:(http2:settings max_frame_size:16384)
00000000  00 00 06 04 00 00 00 00  00 00 05 00 00 40 00     |.............@.|
grpc:(http2:settings:ack)
00000000  00 00 00 04 01 00 00 00  00                       |.........|

20:05:47.203 from CLIENT [1]
grpc:(http2:settings)
00000000  00 00 00 04 00 00 00 00  00                       |.........|

20:05:47.203 from SERVER [1]
grpc:(http2:window_update window_size_increment:57)
00000000  00 00 04 08 00 00 00 00  00 00 00 00 39           |............9|
grpc:(http2:ping 02041010090e0707)
00000000  00 00 08 06 00 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|

20:05:47.203 from CLIENT [1]
grpc:(http2:settings:ack)
00000000  00 00 00 04 01 00 00 00  00                       |.........|
grpc:(http2:headers stream:1 end_headers)
00000000  00 00 4e 01 04 00 00 00  01 83 86 45 97 60 f6 48  |..N........E.`.H|
00000010  5b 17 d5 64 85 b3 40 ea  1c c5 a4 b5 25 81 c9 26  |[..d..@.....%..&|
00000020  ab 24 2d 9f 41 8a 08 9d  5c 0b 81 70 dc 7c 00 7b  |.$-.A...\..p.|.{|
00000030  5f 8b 1d 75 d0 62 0d 26  3d 4c 4d 65 64 7a 8d 9a  |_..u.b.&=LMedz..|
00000040  ca c8 b4 c7 60 2b b4 d2  e1 5a 42 f7 40 02 74 65  |....`+...ZB.@.te|
00000050  86 4d 83 35 05 b1 1f                              |.M.5...|

:method: POST
:scheme: http
:path: /order.OrderManagement/addOrder
:authority: 127.0.0.1:9008
content-type: application/grpc
user-agent: grpc-go/1.44.1-dev
te: trailers

20:05:47.204 from CLIENT [1]
grpc:(http2:data stream:1 len:57 end_stream)
00000000  00 00 39 00 01 00 00 00  01 00 00 00 00 34 0a 03  |..9..........4..|
00000010  31 30 31 12 09 69 50 68  6f 6e 65 20 58 53 12 0c  |101..iPhone XS..|
00000020  4d 61 63 20 42 6f 6f 6b  20 50 72 6f 25 00 c0 0f  |Mac Book Pro%...|
00000030  45 2a 0f 0a 0d 54 6f 6d  2c 20 53 68 61 6e 67 68  |E*...Tom, Shangh|
00000040  61 69                                             |ai|

#1: 101
#2:
  #13: (fixed64)
#2: Mac Book Pro
#4: (fixed32)
#5:
  #1: Tom, Shanghai

20:05:47.204 from CLIENT [1]
grpc:(http2:ping:ack 02041010090e0707)
00000000  00 00 08 06 01 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|

20:05:47.204 from SERVER [1]
grpc:(http2:headers stream:1 end_headers)
00000000  00 00 0e 01 04 00 00 00  01 88 5f 8b 1d 75 d0 62  |.........._..u.b|
00000010  0d 26 3d 4c 4d 65 64                              |.&=LMed|

:status: 200
content-type: application/grpc

grpc:(http2:data stream:1 len:23)
00000000  00 00 17 00 00 00 00 00  01 00 00 00 00 12 0a 10  |................|
00000010  4f 72 64 65 72 20 41 64  64 65 64 3a 20 31 30 31  |Order Added: 101|
grpc:(http2:headers stream:1 end_stream)
00000000  00 00 18 01 05 00 00 00  01 40 88 9a ca c8 b2 12  |.........@......|
00000010  34 da 8f 01 30 40 89 9a  ca c8 b5 25 42 07 31 7f  |4...0@.....%B.1.|
00000020  00                                                |.|

grpc-status: 0
grpc-message:

20:05:47.204 from CLIENT [1]
grpc:(http2:window_update window_size_increment:23)
00000000  00 00 04 08 00 00 00 00  00 00 00 00 17           |.............|
grpc:(http2:ping 02041010090e0707)
00000000  00 00 08 06 00 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|
grpc:(http2:headers stream:3 end_headers)
00000000  00 00 1f 01 04 00 00 00  03 83 86 45 97 60 f6 48  |...........E.`.H|
00000010  5b 17 d5 64 85 b3 40 ea  1c c5 a4 b5 25 89 8a 9d  |[..d..@.....%...|
00000020  56 48 5b 3f c2 c1 c0 bf                           |VH[?....|
grpc:(http2:data stream:3 len:10 end_stream)
00000000  00 00 0a 00 01 00 00 00  03 00 00 00 00 05 0a 03  |................|
00000010  31 30 31                                          |101|

#1: 101

20:05:47.204 from SERVER [1]
grpc:(http2:ping:ack 02041010090e0707)
00000000  00 00 08 06 01 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|
grpc:(http2:window_update window_size_increment:10)
00000000  00 00 04 08 00 00 00 00  00 00 00 00 0a           |.............|
grpc:(http2:ping 02041010090e0707)
00000000  00 00 08 06 00 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|
grpc:(http2:headers stream:3 end_headers)
00000000  00 00 02 01 04 00 00 00  03 88 c0                 |...........|
grpc:(http2:data stream:3 len:57)
00000000  00 00 39 00 00 00 00 00  03 00 00 00 00 34 0a 03  |..9..........4..|
00000010  31 30 31 12 09 69 50 68  6f 6e 65 20 58 53 12 0c  |101..iPhone XS..|
00000020  4d 61 63 20 42 6f 6f 6b  20 50 72 6f 25 00 c0 0f  |Mac Book Pro%...|
00000030  45 2a 0f 0a 0d 54 6f 6d  2c 20 53 68 61 6e 67 68  |E*...Tom, Shangh|
00000040  61 69                                             |ai|
grpc:(http2:headers stream:3 end_stream)
00000000  00 00 02 01 05 00 00 00  03 bf be                 |...........|

20:05:47.204 from SERVER [1]
grpc:(http2:ping:ack 02041010090e0707)
00000000  00 00 08 06 01 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|

20:05:47.204 from CLIENT [1]
grpc:(http2:ping:ack 02041010090e0707)
00000000  00 00 08 06 01 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|
grpc:(http2:window_update window_size_increment:57)
00000000  00 00 04 08 00 00 00 00  00 00 00 00 39           |............9|
grpc:(http2:ping 02041010090e0707)
00000000  00 00 08 06 00 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|

20:05:47.205 [1] CLIENT error, read: connection reset by peer
20:05:47.205 [1] Client connection closed
20:05:47.205 [1] Server connection closed

```

## 客户端流模式
- 服务端

在这段程序中，我们对每一个 Recv 都进行了处理

当发现 io.EOF (流关闭) 后，需要将最终的响应结果发送给客户端，同时关闭正在另外一侧等待的 Recv


```go
func (s *OrderManagementImpl) UpdateOrders(stream pb.OrderManagement_UpdateOrdersServer) error {
	log.Println("UpdateOrders:")
	ordersStr := "Updated Order IDs : "
	for {
		order, err := stream.Recv()
		if err == io.EOF {
			// Finished reading the order stream.
			return stream.SendAndClose(
				&wrapperspb.StringValue{Value: "Orders processed " + ordersStr})
		}
		// Update order
		orders[order.Id] = *order

		log.Println("Order ID ", order.Id, ": Updated")
		ordersStr += order.Id + ", "
	}
}
```

- tproxy观察结果

客户端请求全部在同一 stream:1

```shell
(base) ➜  GolandProjects tproxy -p 9008 -r localhost:9009 -t grpc
20:13:02.839 Listening on 127.0.0.1:9008...
20:13:15.072 [1] Accepted from: 127.0.0.1:53651
20:13:15.073 [1] Connected to server: 127.0.0.1:9009

20:13:15.073 from CLIENT [1]
grpc:(http2:preface)
00000000  50 52 49 20 2a 20 48 54  54 50 2f 32 2e 30 0d 0a  |PRI * HTTP/2.0..|
00000010  0d 0a 53 4d 0d 0a 0d 0a                           |..SM....|

20:13:15.073 from CLIENT [1]
grpc:(http2:settings)
00000000  00 00 00 04 00 00 00 00  00                       |.........|

20:13:15.073 from SERVER [1]
grpc:(http2:settings max_frame_size:16384)
00000000  00 00 06 04 00 00 00 00  00 00 05 00 00 40 00     |.............@.|

20:13:15.073 from SERVER [1]
grpc:(http2:settings:ack)
00000000  00 00 00 04 01 00 00 00  00                       |.........|

20:13:15.073 from CLIENT [1]
grpc:(http2:settings:ack)
00000000  00 00 00 04 01 00 00 00  00                       |.........|
grpc:(http2:headers stream:1 end_headers)
00000000  00 00 50 01 04 00 00 00  01 83 86 45 99 60 f6 48  |..P........E.`.H|
00000010  5b 17 d5 64 85 b3 40 ea  1c c5 a4 b5 25 8b 6b 90  |[..d..@.....%.k.|
00000020  69 2e ab 24 2d 88 41 8a  08 9d 5c 0b 81 70 dc 7c  |i..$-.A...\..p.||
00000030  00 7b 5f 8b 1d 75 d0 62  0d 26 3d 4c 4d 65 64 7a  |.{_..u.b.&=LMedz|
00000040  8d 9a ca c8 b4 c7 60 2b  b4 d2 e1 5a 42 f7 40 02  |......`+...ZB.@.|
00000050  74 65 86 4d 83 35 05 b1  1f                       |te.M.5...|

:method: POST
:scheme: http
:path: /order.OrderManagement/updateOrders
:authority: 127.0.0.1:9008
content-type: application/grpc
user-agent: grpc-go/1.44.1-dev
te: trailers

20:13:15.073 from CLIENT [1]
grpc:(http2:data stream:1 len:57)
00000000  00 00 39 00 00 00 00 00  01 00 00 00 00 34 0a 03  |..9..........4..|
00000010  31 30 31 12 04 4f 70 70  6f 12 06 73 61 6d 75 6e  |101..Oppo..samun|
00000020  67 1a 08 41 20 77 69 74  68 20 42 25 00 00 16 45  |g..A with B%...E|
00000030  2a 10 0a 0e 4d 69 6b 65  2c 20 53 68 61 6e 67 68  |*...Mike, Shangh|
00000040  61 69                                             |ai|
grpc:(http2:data stream:1 len:68)
00000000  00 00 44 00 00 00 00 00  01 00 00 00 00 3f 0a 03  |..D..........?..|
00000010  31 30 32 12 09 69 50 68  6f 6e 65 20 58 53 12 0c  |102..iPhone XS..|
00000020  4d 61 63 20 42 6f 6f 6b  20 50 72 6f 1a 08 43 20  |Mac Book Pro..C |
00000030  77 69 74 68 20 44 25 00  40 1c 45 2a 10 0a 0c 53  |with D%.@.E*...S|
00000040  61 6d 2c 20 42 65 69 6a  69 6e 67 10 01           |am, Beijing..|
grpc:(http2:data stream:1 len:0 end_stream)
00000000  00 00 00 00 01 00 00 00  01                       |.........|

20:13:15.074 from CLIENT [1]
grpc:(http2:ping:ack 02041010090e0707)
00000000  00 00 08 06 01 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|

20:13:15.075 [1] Client connection closed
20:13:15.075 [1] Server connection closed
20:13:15.073 from SERVER [1]
grpc:(http2:window_update window_size_increment:57)
00000000  00 00 04 08 00 00 00 00  00 00 00 00 39           |............9|
grpc:(http2:ping 02041010090e0707)
00000000  00 00 08 06 00 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|

20:13:15.075 from CLIENT [1]
grpc:(http2:window_update window_size_increment:54)
00000000  00 00 04 08 00 00 00 00  00 00 00 00 36           |............6|
grpc:(http2:ping 02041010090e0707)
00000000  00 00 08 06 00 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|

20:13:15.075 from SERVER [1]
grpc:(http2:headers stream:1 end_headers)
00000000  00 00 0e 01 04 00 00 00  01 88 5f 8b 1d 75 d0 62  |.........._..u.b|
00000010  0d 26 3d 4c 4d 65 64                              |.&=LMed|

:status: 200
content-type: application/grpc

grpc:(http2:data stream:1 len:54)
00000000  00 00 36 00 00 00 00 00  01 00 00 00 00 31 0a 2f  |..6..........1./|
00000010  4f 72 64 65 72 73 20 70  72 6f 63 65 73 73 65 64  |Orders processed|
00000020  20 55 70 64 61 74 65 64  20 4f 72 64 65 72 20 49  | Updated Order I|
00000030  44 73 20 3a 20 31 30 31  2c 20 31 30 32 2c 20     |Ds : 101, 102, |
grpc:(http2:headers stream:1 end_stream)
00000000  00 00 18 01 05 00 00 00  01 40 88 9a ca c8 b2 12  |.........@......|
00000010  34 da 8f 01 30 40 89 9a  ca c8 b5 25 42 07 31 7f  |4...0@.....%B.1.|
00000020  00                                                |.|

grpc-status: 0
grpc-message:
```

- 客户端

```go
func clientStream(c pb.OrderManagementClient, ctx context.Context) {
	stream, err := c.UpdateOrders(ctx)
	if err != nil {
		panic(err)
	}

	if err := stream.Send(&pb.Order{
		Id:          "101",
		Items:       []string{"Oppo", "samung"},
		Description: "A with B",
		Price:       2400,
		Addresses: []*pb.Order_Address{
			{
				Address: "Mike, Shanghai",
				Type:    pb.Order_HOME,
			},
		},
	}); err != nil {
		panic(err)
	}

	if err := stream.Send(&pb.Order{
		Id:          "102",
		Items:       []string{"iPhone XS", "Mac Book Pro"},
		Description: "C with D",
		Price:       2500,
		Addresses: []*pb.Order_Address{
			{
				Address: "Sam, Beijing",
				Type:    pb.Order_WORK,
			},
		},
	}); err != nil {
		panic(err)
	}

	res, err := stream.CloseAndRecv()
	if err != nil {
		panic(err)
	}

	log.Printf("Update Orders Res : %s", res)
}
```

## 服务端流模式

- 服务端

```go
func (c *orderManagementClient) SearchOrders(ctx context.Context, in *wrapperspb.StringValue, opts ...grpc.CallOption) (OrderManagement_SearchOrdersClient, error) {
	stream, err := c.cc.NewStream(ctx, &OrderManagement_ServiceDesc.Streams[0], "/order.OrderManagement/searchOrders", opts...)
	if err != nil {
		return nil, err
	}
	x := &orderManagementSearchOrdersClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

```

- 客户端

```go

func serverStream(c pb.OrderManagementClient, ctx context.Context) {
	stream, err := c.SearchOrders(ctx, &wrapperspb.StringValue{Value: "iPhone"})
	if err != nil {
		panic(err)
	}

	for {
		order, err := stream.Recv()
		if err == io.EOF {
			break
		}
		log.Println("Search Result: ", order)
	}
}
```

- tproxy观察结果

```shell
(base) ➜  GolandProjects tproxy -p 9008 -r localhost:9009 -t grpc
20:17:39.052 Listening on 127.0.0.1:9008...
20:17:58.767 [1] Accepted from: 127.0.0.1:54195
20:17:58.768 [1] Connected to server: 127.0.0.1:9009

20:17:58.769 from CLIENT [1]
grpc:(http2:preface)
00000000  50 52 49 20 2a 20 48 54  54 50 2f 32 2e 30 0d 0a  |PRI * HTTP/2.0..|
00000010  0d 0a 53 4d 0d 0a 0d 0a                           |..SM....|

20:17:58.769 from CLIENT [1]
grpc:(http2:settings)
00000000  00 00 00 04 00 00 00 00  00                       |.........|

20:17:58.769 from SERVER [1]
grpc:(http2:settings max_frame_size:16384)
00000000  00 00 06 04 00 00 00 00  00 00 05 00 00 40 00     |.............@.|

20:17:58.769 from SERVER [1]
grpc:(http2:settings:ack)
00000000  00 00 00 04 01 00 00 00  00                       |.........|

20:17:58.769 from CLIENT [1]
grpc:(http2:settings:ack)
00000000  00 00 00 04 01 00 00 00  00                       |.........|
grpc:(http2:headers stream:1 end_headers)
00000000  00 00 50 01 04 00 00 00  01 83 86 45 99 60 f6 48  |..P........E.`.H|
00000010  5b 17 d5 64 85 b3 40 ea  1c c5 a4 b5 25 84 14 76  |[..d..@.....%..v|
00000020  12 7d 56 48 5b 11 41 8a  08 9d 5c 0b 81 70 dc 7c  |.}VH[.A...\..p.||
00000030  00 7b 5f 8b 1d 75 d0 62  0d 26 3d 4c 4d 65 64 7a  |.{_..u.b.&=LMedz|
00000040  8d 9a ca c8 b4 c7 60 2b  b4 d2 e1 5a 42 f7 40 02  |......`+...ZB.@.|
00000050  74 65 86 4d 83 35 05 b1  1f                       |te.M.5...|

:method: POST
:scheme: http
:path: /order.OrderManagement/searchOrders
:authority: 127.0.0.1:9008
content-type: application/grpc
user-agent: grpc-go/1.44.1-dev
te: trailers

20:17:58.769 from CLIENT [1]
grpc:(http2:data stream:1 len:13 end_stream)
00000000  00 00 0d 00 01 00 00 00  01 00 00 00 00 08 0a 06  |................|
00000010  69 50 68 6f 6e 65                                 |iPhone|

#1: iPhone

20:17:58.769 from SERVER [1]
grpc:(http2:window_update window_size_increment:13)
00000000  00 00 04 08 00 00 00 00  00 00 00 00 0d           |.............|
grpc:(http2:ping 02041010090e0707)
00000000  00 00 08 06 00 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|

20:17:58.769 from SERVER [1]
grpc:(http2:headers stream:1 end_headers)
00000000  00 00 0e 01 04 00 00 00  01 88 5f 8b 1d 75 d0 62  |.........._..u.b|
00000010  0d 26 3d 4c 4d 65 64                              |.&=LMed|

:status: 200
content-type: application/grpc

grpc:(http2:data stream:1 len:68)
00000000  00 00 44 00 00 00 00 00  01 00 00 00 00 3f 0a 03  |..D..........?..|
00000010  31 30 32 12 09 69 50 68  6f 6e 65 20 58 53 12 0c  |102..iPhone XS..|
00000020  4d 61 63 20 42 6f 6f 6b  20 50 72 6f 1a 08 43 20  |Mac Book Pro..C |
00000030  77 69 74 68 20 44 25 00  40 1c 45 2a 10 0a 0c 53  |with D%.@.E*...S|
00000040  61 6d 2c 20 42 65 69 6a  69 6e 67 10 01           |am, Beijing..|
grpc:(http2:headers stream:1 end_stream)
00000000  00 00 18 01 05 00 00 00  01 40 88 9a ca c8 b2 12  |.........@......|
00000010  34 da 8f 01 30 40 89 9a  ca c8 b5 25 42 07 31 7f  |4...0@.....%B.1.|
00000020  00                                                |.|

grpc-status: 0
grpc-message:

20:17:58.769 from CLIENT [1]
grpc:(http2:ping:ack 02041010090e0707)
00000000  00 00 08 06 01 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|

20:17:58.770 from CLIENT [1]
grpc:(http2:window_update window_size_increment:68)
00000000  00 00 04 08 00 00 00 00  00 00 00 00 44           |............D|
grpc:(http2:ping 02041010090e0707)
00000000  00 00 08 06 00 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|

20:17:58.770 from SERVER [1]
grpc:(http2:ping:ack 02041010090e0707)
00000000  00 00 08 06 01 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|

20:17:58.770 [1] CLIENT error, read: connection reset by peer
20:17:58.770 [1] Client connection closed
20:17:58.770 [1] Server connection closed
```




## 双向流模式

- 服务端

```go

// ProcessOrders process orders
func (s *OrderManagementImpl) ProcessOrders(stream pb.OrderManagement_ProcessOrdersServer) error {
	log.Println("ProcessOrders:")
	batchMarker := 1
	var combinedShipmentMap = make(map[string]pb.CombinedShipment)
	for {
		orderId, err := stream.Recv()
		if err == io.EOF {
			for _, shipment := range combinedShipmentMap {
				if err := stream.Send(&shipment); err != nil {
					return err
				}
			}
			return nil
		}
		if err != nil {
			log.Println(err)
			return err
		}

		destination := orders[orderId.GetValue()].Addresses[0].Address
		shipment, found := combinedShipmentMap[destination]

		if found {
			ord := orders[orderId.GetValue()]
			shipment.OrderList = append(shipment.OrderList, &ord)
			combinedShipmentMap[destination] = shipment
		} else {
			comShip := pb.CombinedShipment{Id: "cmb - " + (orders[orderId.GetValue()].Addresses[0].Address), Status: "Processed!"}
			ord := orders[orderId.GetValue()]
			comShip.OrderList = append(shipment.OrderList, &ord)
			combinedShipmentMap[destination] = comShip
			log.Print(len(comShip.OrderList), comShip.GetId())
		}

		if batchMarker == orderBatchSize {
			for _, comb := range combinedShipmentMap {
				log.Printf("Shipping : %v -> %v", comb.Id, len(comb.OrderList))
				if err := stream.Send(&comb); err != nil {
					return err
				}
			}
			batchMarker = 0
			combinedShipmentMap = make(map[string]pb.CombinedShipment)
		} else {
			batchMarker++
		}
	}
}
```

- 客户端

```go

func bidirectionalStream(c pb.OrderManagementClient, ctx context.Context) {
	stream, err := c.ProcessOrders(ctx)
	if err != nil {
		panic(err)
	}

	go func() {
		if err := stream.Send(&wrapperspb.StringValue{Value: "101"}); err != nil {
			panic(err)
		}

		if err := stream.Send(&wrapperspb.StringValue{Value: "102"}); err != nil {
			panic(err)
		}

		if err := stream.CloseSend(); err != nil {
			panic(err)
		}
	}()

	for {
		combinedShipment, err := stream.Recv()
		if err == io.EOF {
			break
		}
		log.Println("Combined shipment : ", combinedShipment.OrderList)
	}
}
```

- tproxy观察结果

客户端和服务端在同一个 stream:1

```shell
(base) ➜  GolandProjects tproxy -p 9008 -r localhost:9009 -t grpc
20:21:30.708 Listening on 127.0.0.1:9008...
20:21:41.480 [1] Accepted from: 127.0.0.1:54609
20:21:41.481 [1] Connected to server: 127.0.0.1:9009

20:21:41.481 from CLIENT [1]
grpc:(http2:preface)
00000000  50 52 49 20 2a 20 48 54  54 50 2f 32 2e 30 0d 0a  |PRI * HTTP/2.0..|
00000010  0d 0a 53 4d 0d 0a 0d 0a                           |..SM....|

20:21:41.481 from CLIENT [1]
grpc:(http2:settings)
00000000  00 00 00 04 00 00 00 00  00                       |.........|

20:21:41.481 from SERVER [1]
grpc:(http2:settings max_frame_size:16384)
00000000  00 00 06 04 00 00 00 00  00 00 05 00 00 40 00     |.............@.|

20:21:41.481 from SERVER [1]
grpc:(http2:settings:ack)
00000000  00 00 00 04 01 00 00 00  00                       |.........|

20:21:41.481 from CLIENT [1]
grpc:(http2:settings:ack)
00000000  00 00 00 04 01 00 00 00  00                       |.........|

20:21:41.482 from CLIENT [1]
grpc:(http2:headers stream:1 end_headers)
00000000  00 00 51 01 04 00 00 00  01 83 86 45 9a 60 f6 48  |..Q........E.`.H|
00000010  5b 17 d5 64 85 b3 40 ea  1c c5 a4 b5 25 8a ec 39  |[..d..@.....%..9|
00000020  0a 84 6a b2 42 d8 8f 41  8a 08 9d 5c 0b 81 70 dc  |..j.B..A...\..p.|
00000030  7c 00 7b 5f 8b 1d 75 d0  62 0d 26 3d 4c 4d 65 64  ||.{_..u.b.&=LMed|
00000040  7a 8d 9a ca c8 b4 c7 60  2b b4 d2 e1 5a 42 f7 40  |z......`+...ZB.@|
00000050  02 74 65 86 4d 83 35 05  b1 1f                    |.te.M.5...|

:method: POST
:scheme: http
:path: /order.OrderManagement/processOrders
:authority: 127.0.0.1:9008
content-type: application/grpc
user-agent: grpc-go/1.44.1-dev
te: trailers

20:21:41.482 from CLIENT [1]
grpc:(http2:data stream:1 len:10)
00000000  00 00 0a 00 00 00 00 00  01 00 00 00 00 05 0a 03  |................|
00000010  31 30 31                                          |101|
grpc:(http2:data stream:1 len:10)
00000000  00 00 0a 00 00 00 00 00  01 00 00 00 00 05 0a 03  |................|
00000010  31 30 32                                          |102|
grpc:(http2:data stream:1 len:0 end_stream)
00000000  00 00 00 00 01 00 00 00  01                       |.........|

20:21:41.483 from CLIENT [1]
grpc:(http2:ping:ack 02041010090e0707)
00000000  00 00 08 06 01 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|

20:21:41.483 [1] Client connection closed
20:21:41.483 [1] Server connection closed
20:21:41.483 from CLIENT [1]
grpc:(http2:window_update window_size_increment:93)
00000000  00 00 04 08 00 00 00 00  00 00 00 00 5d           |............]|
grpc:(http2:ping 02041010090e0707)
00000000  00 00 08 06 00 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|

20:21:41.482 from SERVER [1]
grpc:(http2:window_update window_size_increment:10)
00000000  00 00 04 08 00 00 00 00  00 00 00 00 0a           |.............|
grpc:(http2:ping 02041010090e0707)
00000000  00 00 08 06 00 00 00 00  00 02 04 10 10 09 0e 07  |................|
00000010  07                                                |.|

20:21:41.483 from SERVER [1]
grpc:(http2:headers stream:1 end_headers)
00000000  00 00 0e 01 04 00 00 00  01 88 5f 8b 1d 75 d0 62  |.........._..u.b|
00000010  0d 26 3d 4c 4d 65 64                              |.&=LMed|

:status: 200
content-type: application/grpc

grpc:(http2:data stream:1 len:93)
00000000  00 00 5d 00 00 00 00 00  01 00 00 00 00 58 0a 14  |..]..........X..|
00000010  63 6d 62 20 2d 20 4d 69  6b 65 2c 20 53 68 61 6e  |cmb - Mike, Shan|
00000020  67 68 61 69 12 0a 50 72  6f 63 65 73 73 65 64 21  |ghai..Processed!|
00000030  1a 34 0a 03 31 30 31 12  04 4f 70 70 6f 12 06 73  |.4..101..Oppo..s|
00000040  61 6d 75 6e 67 1a 08 41  20 77 69 74 68 20 42 25  |amung..A with B%|
00000050  00 00 16 45 2a 10 0a 0e  4d 69 6b 65 2c 20 53 68  |...E*...Mike, Sh|
00000060  61 6e 67 68 61 69                                 |anghai|
grpc:(http2:data stream:1 len:102)
00000000  00 00 66 00 00 00 00 00  01 00 00 00 00 61 0a 12  |..f..........a..|
00000010  63 6d 62 20 2d 20 53 61  6d 2c 20 42 65 69 6a 69  |cmb - Sam, Beiji|
00000020  6e 67 12 0a 50 72 6f 63  65 73 73 65 64 21 1a 3f  |ng..Processed!.?|
00000030  0a 03 31 30 32 12 09 69  50 68 6f 6e 65 20 58 53  |..102..iPhone XS|
00000040  12 0c 4d 61 63 20 42 6f  6f 6b 20 50 72 6f 1a 08  |..Mac Book Pro..|
00000050  43 20 77 69 74 68 20 44  25 00 40 1c 45 2a 10 0a  |C with D%.@.E*..|
00000060  0c 53 61 6d 2c 20 42 65  69 6a 69 6e 67 10 01     |.Sam, Beijing..|
grpc:(http2:headers stream:1 end_stream)
00000000  00 00 18 01 05 00 00 00  01 40 88 9a ca c8 b2 12  |.........@......|
00000010  34 da 8f 01 30 40 89 9a  ca c8 b5 25 42 07 31 7f  |4...0@.....%B.1.|
00000020  00                                                |.|

grpc-status: 0
grpc-message:

```
