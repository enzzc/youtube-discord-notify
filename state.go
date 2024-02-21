package main

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type State interface {
	Set(string) error
	Get() (string, error)
}

type FileState struct {
	path string
}

func NewFileState(path string) *FileState {
	_, err := os.Stat(path)
	if err != nil {
		os.Create(path)
	}
	return &FileState{
		path: path,
	}
}

func (s *FileState) Set(value string) error {
	return os.WriteFile(s.path, []byte(value), 0644)
}

func (s *FileState) Get() (string, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type DynamoDBState struct {
	client *dynamodb.Client
	table  *string
}

func NewDynamoDBStoreWithEnvCreds(table string) *DynamoDBState {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("eu-west-3"),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
				SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
				Source:          "From secret env.",
			},
		}),
	)
	if err != nil {
		return nil
	}
	client := dynamodb.NewFromConfig(cfg)
	return &DynamoDBState{
		client: client,
		table:  aws.String(table),
	}
}

func (s *DynamoDBState) Set(value string) error {
	attrs := map[string]string{
		"linkID": "latest",
		"link":   value,
	}
	av, _ := attributevalue.MarshalMap(attrs)
	input := &dynamodb.PutItemInput{
		TableName: s.table,
		Item:      av,
	}
	_, err := s.client.PutItem(context.TODO(), input)
	return err
}

func (s *DynamoDBState) Get() (string, error) {
	keys := map[string]string{
		"linkID": "latest",
	}
	key, _ := attributevalue.MarshalMap(keys)
	input := &dynamodb.GetItemInput{
		TableName:       s.table,
		AttributesToGet: []string{"link"},
		Key:             key,
	}
	itemOutput, err := s.client.GetItem(context.TODO(), input)
	if err != nil {
		return "", err
	}
	var item map[string]string
	attributevalue.UnmarshalMap(itemOutput.Item, &item)
	link, ok := item["link"]
	if !ok {
		return "", nil
	}
	return link, nil
}
