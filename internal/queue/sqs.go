package queue

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"log"
	"strconv"
	"time"
)

var _ Queue = (*sqsQueue)(nil)

type sqsQueue struct {
	queueURL          string
	sqsClient         *sqs.Client
	pollInterval      time.Duration
	visibilityTimeout time.Duration
}

type SQSConfig struct {
	QueueURL          string
	Client            *sqs.Client
	PollInterval      time.Duration
	VisibilityTimeout time.Duration
}

func NewSQSQueue(cfg *SQSConfig) Queue {
	return &sqsQueue{
		queueURL:          cfg.QueueURL,
		sqsClient:         cfg.Client,
		pollInterval:      cfg.PollInterval,
		visibilityTimeout: cfg.VisibilityTimeout,
	}
}

// Push pushes an event to the queue
// delay is in seconds
func (q *sqsQueue) Push(ctx context.Context, event *Event, delay int64) error {
	out, err := q.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		MessageBody:  aws.String(string(event.Data)),
		QueueUrl:     aws.String(q.queueURL),
		DelaySeconds: int32(delay),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"id": {
				DataType:    aws.String("String"),
				StringValue: aws.String(event.ID),
			},
			"name": {
				DataType:    aws.String("String"),
				StringValue: aws.String(string(event.Name)),
			},
		},
		MessageDeduplicationId: aws.String(event.ID),
		MessageGroupId:         aws.String(string(event.Name)),
	})
	if err != nil {
		return err
	}

	log.Println("pushed event to queue: ", aws.ToString(out.MessageId))

	// TODO: should we return the message ID?

	return nil
}

func (q *sqsQueue) Pop(ctx context.Context, size int64) ([]*Event, error) {
	resp, err := q.sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:                aws.String(q.queueURL),
		MaxNumberOfMessages:     int32(size),
		ReceiveRequestAttemptId: aws.String(fmt.Sprintf("%d", time.Now().UnixNano())),
		AttributeNames: []types.QueueAttributeName{
			"MessageDeduplicationId",
			"ApproximateReceiveCount",
		},
		MessageAttributeNames: []string{
			"id",
			"name",
		},
		VisibilityTimeout: int32(q.visibilityTimeout.Seconds()),
		WaitTimeSeconds:   int32(q.pollInterval.Seconds()),
	})
	if err != nil {
		return nil, err
	}

	log.Println(len(resp.Messages), "messages from queue")

	if len(resp.Messages) == 0 {
		return nil, nil
	}

	var events []*Event

	for _, msg := range resp.Messages {
		var retries int

		if msg.Attributes["ApproximateReceiveCount"] != "" {
			retries, _ = strconv.Atoi(msg.Attributes["ApproximateReceiveCount"])
		}

		events = append(events, &Event{
			sqsReceiptHandle: aws.ToString(msg.ReceiptHandle),

			ID:         aws.ToString(msg.MessageAttributes["id"].StringValue),
			Name:       EventName(aws.ToString(msg.MessageAttributes["name"].StringValue)),
			Data:       []byte(aws.ToString(msg.Body)),
			RetryCount: retries,
		})
	}

	return events, nil
}

func (q *sqsQueue) Retry(ctx context.Context, event *Event) error {
	_, err := q.sqsClient.ChangeMessageVisibility(ctx, &sqs.ChangeMessageVisibilityInput{
		QueueUrl:          aws.String(q.queueURL),
		ReceiptHandle:     aws.String(event.sqsReceiptHandle),
		VisibilityTimeout: 0,
	})
	if err != nil {
		return err
	}

	return nil
}

func (q *sqsQueue) Remove(ctx context.Context, event *Event) error {
	_, err := q.sqsClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(q.queueURL),
		ReceiptHandle: aws.String(event.sqsReceiptHandle),
	})
	if err != nil {
		return err
	}

	return nil
}
