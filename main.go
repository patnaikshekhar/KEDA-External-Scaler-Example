package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"

	log "github.com/Sirupsen/logrus"
	"github.com/go-redis/redis"
	empty "github.com/golang/protobuf/ptypes/empty"
	pb "github.com/patnaikshekhar/keda_external_scaler/externalscaler"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	listLengthMetricName    = "RedisListLength"
	defaultTargetListLength = 5
	defaultRedisAddress     = "redis-master.default.svc.cluster.local:6379"
	defaultRedisPassword    = ""
	port                    = 8080
)

func main() {

	lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		panic(err)
	}

	certBasePath := os.Getenv("CERT_PATH")
	certFile := fmt.Sprintf("%s/server.crt", certBasePath)
	keyFile := fmt.Sprintf("%s/server.key", certBasePath)

	creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
	if err != nil {
		panic(err)
	}

	server := grpc.NewServer(grpc.Creds(creds))
	pb.RegisterExternalScalerServer(server, &RedisExternalScalerServer{})
	log.Println("Starting server")
	server.Serve(lis)
}

// RedisExternalScalerServer implements the redis scaler as a GRPC server
type RedisExternalScalerServer struct {
	scalers map[string]*RedisScaler
}

// RedisScaler is a single instance that handles redis scaling
type RedisScaler struct {
	address    string
	password   string
	listName   string
	listLength int
}

func getScalerUniqueName(scaledObjectRef *pb.ScaledObjectRef) string {
	return scaledObjectRef.Namespace + "/" + scaledObjectRef.Name
}

// New creates a new instance of a redis scaler
func (s *RedisExternalScalerServer) New(ctx context.Context, request *pb.NewRequest) (*empty.Empty, error) {

	if s.scalers == nil {
		s.scalers = make(map[string]*RedisScaler)
	}

	name := getScalerUniqueName(request.ScaledObjectRef)
	log.Printf("New() method called for %s", name)

	scaler, err := parseRedisMetadata(request.Metadata)
	if err != nil {
		return nil, err
	}

	s.scalers[name] = scaler

	log.Printf("New() method completed for %s", name)

	return &empty.Empty{}, nil
}

// Close creates a new instance of a redis scaler
func (s *RedisExternalScalerServer) Close(ctx context.Context, request *pb.ScaledObjectRef) (*empty.Empty, error) {

	name := getScalerUniqueName(request)
	log.Printf("Close() method called for %s", name)

	if _, ok := s.scalers[name]; ok {
		delete(s.scalers, name)
	}

	log.Printf("Close() method completed for %s", name)

	return &empty.Empty{}, nil
}

func parseRedisMetadata(metadata map[string]string) (*RedisScaler, error) {
	scaler := RedisScaler{}
	scaler.listLength = defaultTargetListLength

	if val, ok := metadata["listLength"]; ok {
		listLength, err := strconv.Atoi(val)
		if err != nil {
			return nil, fmt.Errorf("List length parsing error %s", err.Error())
		}

		scaler.listLength = listLength
	}

	if val, ok := metadata["listName"]; ok {
		scaler.listName = val
	} else {
		return nil, fmt.Errorf("no list name given")
	}

	scaler.address = defaultRedisAddress
	if val, ok := metadata["address"]; ok && val != "" {
		scaler.address = val
	}

	scaler.password = defaultRedisPassword
	if val, ok := metadata["password"]; ok && val != "" {
		scaler.password = val
	}

	return &scaler, nil
}

// IsActive checks if there are any messages in the redis list
func (s *RedisExternalScalerServer) IsActive(ctx context.Context, request *pb.ScaledObjectRef) (*pb.IsActiveResponse, error) {

	name := getScalerUniqueName(request)
	log.Printf("IsActive() method called for %s", name)

	if scalerRef, ok := s.scalers[name]; ok {
		result, err := getRedisListLength(
			ctx, scalerRef.address, scalerRef.password, scalerRef.listName)

		if err != nil {
			return nil, err
		}

		log.Printf("IsActive() method Completed for %s", name)

		return &pb.IsActiveResponse{
			Result: result > 0,
		}, nil

	}

	return nil, fmt.Errorf("Cannot find scaler %s", name)
}

// GetMetricSpec returns the metric name and target average value for the HPA spec
func (s *RedisExternalScalerServer) GetMetricSpec(ctx context.Context, request *pb.ScaledObjectRef) (*pb.GetMetricSpecResponse, error) {

	name := getScalerUniqueName(request)
	log.Printf("GetMetricSpec() method called for %s", name)

	if scalerRef, ok := s.scalers[name]; ok {
		spec := pb.MetricSpec{
			MetricName: listLengthMetricName,
			TargetSize: int64(scalerRef.listLength),
		}

		log.Printf("GetMetricSpec() method completed for %s", name)

		return &pb.GetMetricSpecResponse{
			MetricSpecs: []*pb.MetricSpec{&spec},
		}, nil
	}

	return nil, fmt.Errorf("Cannot find scaler %s", name)
}

// GetMetrics returns the current state of metrics
func (s *RedisExternalScalerServer) GetMetrics(ctx context.Context, request *pb.GetMetricsRequest) (*pb.GetMetricsResponse, error) {

	name := getScalerUniqueName(request.ScaledObjectRef)
	log.Printf("GetMetrics() method called for %s", name)

	if scalerRef, ok := s.scalers[name]; ok {
		listLen, err := getRedisListLength(ctx, scalerRef.address, scalerRef.password, scalerRef.listName)

		if err != nil {
			return nil, err
		}

		value := pb.MetricValue{
			MetricName:  listLengthMetricName,
			MetricValue: listLen,
		}

		log.Printf("GetMetrics() method completed for %s", name)

		return &pb.GetMetricsResponse{
			MetricValues: []*pb.MetricValue{&value},
		}, nil
	}

	return nil, fmt.Errorf("Cannot find scaler %s", name)
}

func getRedisListLength(ctx context.Context, address string, password string, listName string) (int64, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		DB:       0,
	})

	cmd := client.LLen(listName)

	if cmd.Err() != nil {
		return -1, cmd.Err()
	}

	return cmd.Result()
}
