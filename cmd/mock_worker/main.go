package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"

	"github.com/leon37/neuro-grid/internal/server" // 临时引用，实际部署时 Client 和 Server 分离
	"github.com/leon37/neuro-grid/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// 启动一个本地 gRPC Server 方便调试
func startLocalServer() string {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	srv := server.NewNeuroServer()
	pb.RegisterNeuroServiceServer(s, srv)

	go func() {
		log.Println("Neuro-Grid Control Plane listening on :50051")
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	return ":50051"
}

func runMockWorker(id string, address string) {
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewNeuroServiceClient(conn)

	// 1. Register (Login)
	// 随机模拟不同的显卡
	gpuModels := []string{"RTX 4090", "RTX 3090", "A100", "H100"}
	model := gpuModels[rand.Intn(len(gpuModels))]

	_, err = c.RegisterWorker(context.Background(), &pb.WorkerInfo{
		WorkerId:    id,
		GpuModel:    model,
		VramTotalMb: 24576,
	})
	if err != nil {
		log.Printf("Worker %s login failed: %v", id, err)
		return
	}

	// 2. Heartbeat Loop (Tick)
	ticker := time.NewTicker(3 * time.Second)
	for range ticker.C {
		_, err := c.Heartbeat(context.Background(), &pb.Ping{WorkerId: id})
		if err != nil {
			log.Printf("Worker %s lost connection", id)
			return
		}
		// 假装我们在忙...
	}
}

func main() {
	// 启动服务端
	addr := startLocalServer()

	// 模拟集群启动！
	// 启动 5 个 Mock Worker
	for i := 1; i <= 5; i++ {
		workerID := fmt.Sprintf("worker-node-%03d", i)
		go runMockWorker(workerID, addr)
		time.Sleep(500 * time.Millisecond) // 错峰上线
	}

	// 阻塞主进程
	select {}
}
