package broker

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Rabbit struct {
	url           string
	mu            sync.Mutex
	conn          *amqp.Connection
	publisher     *amqp.Channel
	confirmations <-chan amqp.Confirmation
	closed        bool
}
type Message struct {
	JobID       string `json:"job_id"`
	Traceparent string `json:"traceparent,omitempty"`
}

func Open(url string) (*Rabbit, error) {
	r := &Rabbit{url: url}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = r.ensurePublisher(ctx)
	return r, nil
}
func (r *Rabbit) Close() { r.mu.Lock(); defer r.mu.Unlock(); r.closed = true; r.closeLocked() }
func (r *Rabbit) Healthy() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.closed && r.conn != nil && !r.conn.IsClosed() && r.publisher != nil && !r.publisher.IsClosed()
}
func (r *Rabbit) closeLocked() {
	if r.publisher != nil {
		_ = r.publisher.Close()
		r.publisher = nil
	}
	if r.conn != nil {
		_ = r.conn.Close()
		r.conn = nil
	}
	r.confirmations = nil
}
func (r *Rabbit) invalidate() { r.mu.Lock(); defer r.mu.Unlock(); r.closeLocked() }
func (r *Rabbit) connectLocked() error {
	if r.closed {
		return errors.New("rabbitmq client is closed")
	}
	if r.conn != nil && !r.conn.IsClosed() && r.publisher != nil && !r.publisher.IsClosed() {
		return nil
	}
	r.closeLocked()
	conn, err := amqp.Dial(r.url)
	if err != nil {
		return err
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return err
	}
	if err = ch.Confirm(false); err != nil {
		ch.Close()
		conn.Close()
		return err
	}
	r.conn = conn
	r.publisher = ch
	r.confirmations = ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	return nil
}
func (r *Rabbit) ensurePublisher(ctx context.Context) error {
	delay := 100 * time.Millisecond
	for {
		r.mu.Lock()
		err := r.connectLocked()
		r.mu.Unlock()
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		if delay < 5*time.Second {
			delay *= 2
		}
	}
}
func (r *Rabbit) Declare(ctx context.Context, queue string) error {
	if err := r.ensurePublisher(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.publisher.QueueDeclare(queue, true, false, false, false, amqp.Table{"x-max-priority": int32(9)})
	if err != nil {
		r.closeLocked()
	}
	return err
}
func (r *Rabbit) Publish(ctx context.Context, queue, id string, priority int16, traceparent ...string) error {
	trace := ""
	if len(traceparent) > 0 {
		trace = traceparent[0]
	}
	body, _ := json.Marshal(Message{JobID: id, Traceparent: trace})
	for attempts := 0; attempts < 2; attempts++ {
		if err := r.ensurePublisher(ctx); err != nil {
			return err
		}
		r.mu.Lock()
		ch := r.publisher
		confirmations := r.confirmations
		err := ch.PublishWithContext(ctx, "", queue, false, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Priority: uint8(priority), Timestamp: time.Now(), Body: body})
		r.mu.Unlock()
		if err != nil {
			r.invalidate()
			continue
		}
		select {
		case confirmation, ok := <-confirmations:
			if !ok {
				r.invalidate()
				continue
			}
			if !confirmation.Ack {
				return errors.New("rabbitmq negatively acknowledged publication")
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return errors.New("rabbitmq publish failed after reconnect")
}
func (r *Rabbit) Consume(ctx context.Context, queue, tag string, prefetch int) <-chan amqp.Delivery {
	out := make(chan amqp.Delivery)
	go func() {
		defer close(out)
		delay := 100 * time.Millisecond
		for {
			if ctx.Err() != nil {
				return
			}
			deliveries, ch, err := r.openConsumer(queue, tag, prefetch)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(delay):
				}
				if delay < 5*time.Second {
					delay *= 2
				}
				continue
			}
			delay = 100 * time.Millisecond
			for {
				select {
				case <-ctx.Done():
					_ = ch.Close()
					return
				case delivery, ok := <-deliveries:
					if !ok {
						_ = ch.Close()
						r.invalidate()
						goto reconnect
					}
					select {
					case out <- delivery:
					case <-ctx.Done():
						_ = ch.Close()
						return
					}
				}
			}
		reconnect:
		}
	}()
	return out
}
func (r *Rabbit) openConsumer(queue, tag string, prefetch int) (<-chan amqp.Delivery, *amqp.Channel, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.ensurePublisher(ctx); err != nil {
		return nil, nil, err
	}
	r.mu.Lock()
	conn := r.conn
	r.mu.Unlock()
	ch, err := conn.Channel()
	if err != nil {
		r.invalidate()
		return nil, nil, err
	}
	if err = ch.Qos(prefetch, 0, false); err != nil {
		ch.Close()
		return nil, nil, err
	}
	if _, err = ch.QueueDeclare(queue, true, false, false, false, amqp.Table{"x-max-priority": int32(9)}); err != nil {
		ch.Close()
		return nil, nil, err
	}
	deliveries, err := ch.Consume(queue, tag, false, false, false, false, nil)
	if err != nil {
		ch.Close()
		return nil, nil, err
	}
	return deliveries, ch, nil
}
