package main

import (
	"context"
	"io"
	"log"

	pb "github.com/2456868764/k8s-learning/grpc/basic/order"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func main() {
	// 9008 tproxy代理端口，会转发到 9009 grpc 端口
	conn, err := grpc.Dial("127.0.0.1:9008",
		grpc.WithInsecure(),
	)
	//conn, err := grpc.Dial("127.0.0.1:9009",
	//	grpc.WithInsecure(),
	//)
	if err != nil {
		panic(err)
	}

	c := pb.NewOrderManagementClient(conn)

	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	//UnaryRPC(c, ctx)
	//clientStream(c, ctx)
	//serverStream(c, ctx)
	bidirectionalStream(c, ctx)
}

func UnaryRPC(c pb.OrderManagementClient, ctx context.Context) {
	// Add Order
	order := pb.Order{
		Id:    "101",
		Items: []string{"iPhone XS", "Mac Book Pro"},
		Addresses: []*pb.Order_Address{
			{
				Address: "Tom, Shanghai",
				Type:    pb.Order_HOME,
			},
		},
		Price: 2300.00,
	}
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

	//
	//// Add Order
	//order = pb.Order{
	//	Id:    "102",
	//	Items: []string{"Oppo", "Vivo"},
	//	Addresses: []*pb.Order_Address{
	//		{
	//			Address: "Alex, Beijing",
	//			Type:    pb.Order_WORK,
	//		},
	//	},
	//	Price: 2300.00,
	//}
	//res, err = c.AddOrder(ctx, &order)
	//if err != nil {
	//	panic(err)
	//}
	//log.Print("AddOrder Response -> ", res.Value)
	//
	//// Get Order
	//retrievedOrder, err = c.GetOrder(ctx, &wrapperspb.StringValue{Value: "102"})
	//if err != nil {
	//	panic(err)
	//}
	//log.Print("GetOrder Response -> : ", retrievedOrder)
}

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
