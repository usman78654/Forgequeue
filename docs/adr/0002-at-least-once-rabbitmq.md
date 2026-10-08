# ADR 0002: RabbitMQ with at-least-once delivery

Status: accepted

RabbitMQ fits competing-consumer job queues and priority routing. Deliveries are acknowledged only after durable completion state is written. Duplicates remain possible after failures, so claims use PostgreSQL leases and fencing tokens. Exactly-once execution is not promised; callers use idempotency keys and handlers should be idempotent.
