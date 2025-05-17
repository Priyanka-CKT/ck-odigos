package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type PathKey struct {
	PathKey       string        `json:"pathKey"`
	GraphPathNode GraphPathNode `json:"graphPathNode"`
}

type GraphPathNode struct {
	IncomingPath     *string      `json:"incomingPath"`
	GraphPathElement GraphElement `json:"graphPathElement"`
	Runtime          string       `json:"runtime" default:"go"`
	StartTime        uint64
	EndTime          uint64
	IsError          bool
	ErrorCode        string
	EventCount       int //This is the number of events if its a batch event. Else its 1.
}

// GraphElement interface
type GraphElement interface{}

// Extensions
// GrpcServiceElement struct
type GrpcServiceElement struct {
	Type               string `json:"type"`
	GrpcFullMethodName string `json:"grpcFullMethodName"`
}

// HttpServiceElement struct
type HttpServiceElement struct {
	Type       string `json:"type"`
	HttpMethod string `json:"httpMethod"`
	HttpRoute  string `json:"httpRoute"`
}

// KafkaConsumerElement struct
type KafkaConsumerElement struct {
	Type      string `json:"type"`
	GroupId   string `json:"groupId"`
	TopicName string `json:"topicName"`
}

// TODO: these two will be removed later
// SQLElement struct
type SQLElement struct {
	Type      string `json:"type"`
	Url       string `json:"url"`
	QueryType string `json:"queryType"`
}

type RedisElement struct {
	Type    string `json:"type"`
	Command string `json:"commandName"`
	Server  string `json:"server"`
}

// Dynamo DB
type AWSServiceElement struct {
	Type        string `json:"type"`
	ServiceName string `json:"serviceName"`
	Action      string `json:"action"`
}

type SQSConsumerElement struct {
	Type      string `json:"type"`
	QueueName string `json:"queueName"`
}

// ExternalClientElement struct
type ExternalClientElement struct {
	Type        string `json:"type"`
	ClientID    string `json:"clientId"`
	RequestType string `json:"requestType"`
}

// HttpClientBridgeElement struct
type HttpClientBridgeElement struct {
	Type          string `json:"type"`
	HttpMethod    string `json:"httpMethod"`
	HttpScheme    string `json:"httpScheme"`
	ServerPathKey string `json:"serverPathKey"`
}

// GrpcClientBridgeElement struct
type GrpcClientBridgeElement struct {
	Type               string `json:"type"`
	GrpcFullMethodName string `json:"grpcFullMethodName"`
}

// KafkaProducerBridgeElement struct
type KafkaProducerBridgeElement struct {
	Type      string `json:"type"`
	TopicName string `json:"topicName"`
}

// AwsMessagingBridgeElement struct
type AwsMessagingBridgeElement struct {
	Type       string `json:"type"`
	TargetArn  string `json:"targetArn"`
	TargetType string `json:"targetType"`
}

// GeneratePathKey creates a deterministic path key from a list of strings.
// It combines all inputs and generates a 32-character hex string suitable for trace IDs.
// This is used to generate consistent path keys across different probe types.
func GeneratePathKey(components ...string) string {
	// Combine all components with a separator to ensure uniqueness
	combined := strings.Join(components, "|")
	// Create SHA-256 hash
	hash := sha256.Sum256([]byte(combined))
	// Create a new 16-byte array for the path key
	var pathKey [16]byte
	// XOR the 32 bytes of SHA-256 into 16 bytes
	// This preserves randomness while reducing to required size
	for i := 0; i < 16; i++ {
		pathKey[i] = hash[i] ^ hash[i+16]
	}
	// Convert to hex string (32 characters)
	return hex.EncodeToString(pathKey[:])
}
