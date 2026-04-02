package service

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/possibities/gin-core/internal/event"
	"github.com/possibities/gin-core/internal/model"
)

const userProfileUpdatedTopic = "user.profile.updated"
const userProfileUpdatedMQTopic = "user.profile.updated.v1"

type UserProfileUpdatedEvent struct {
	ActorUserID uint
	UserID      uint
	Before      UserProfile
	After       UserProfile
	IPAddress   string
	TraceID     string
}

func (e UserProfileUpdatedEvent) Topic() string {
	return userProfileUpdatedTopic
}

func (e UserProfileUpdatedEvent) OutboxEvent(now time.Time) (*model.OutboxEvent, error) {
	payload, err := json.Marshal(struct {
		ActorUserID uint        `json:"actor_user_id"`
		UserID      uint        `json:"user_id"`
		Before      UserProfile `json:"before"`
		After       UserProfile `json:"after"`
		IPAddress   string      `json:"ip_address"`
		TraceID     string      `json:"trace_id"`
		OccurredAt  time.Time   `json:"occurred_at"`
	}{
		ActorUserID: e.ActorUserID,
		UserID:      e.UserID,
		Before:      e.Before,
		After:       e.After,
		IPAddress:   e.IPAddress,
		TraceID:     e.TraceID,
		OccurredAt:  now.UTC(),
	})
	if err != nil {
		return nil, err
	}

	ts := now.UTC()
	return &model.OutboxEvent{
		ID:            uuid.NewString(),
		Topic:         userProfileUpdatedMQTopic,
		Payload:       payload,
		Status:        model.OutboxStatusPending,
		Attempts:      0,
		NextAttemptAt: ts,
		CreatedAt:     ts,
		UpdatedAt:     ts,
	}, nil
}

type UserProfileUpdatedSubscriber struct{}

func NewUserProfileUpdatedSubscriber(bus *event.Bus, audit *AuditService) *UserProfileUpdatedSubscriber {
	bus.Subscribe(userProfileUpdatedTopic, func(ctx context.Context, message event.Message) error {
		evt, ok := message.(UserProfileUpdatedEvent)
		if !ok {
			return nil
		}

		beforeState, err := json.Marshal(evt.Before)
		if err != nil {
			return err
		}
		afterState, err := json.Marshal(evt.After)
		if err != nil {
			return err
		}

		return audit.Record(ctx, AuditRecord{
			ActorUserID:  evt.ActorUserID,
			Action:       "user.profile.update",
			ResourceType: "user",
			ResourceID:   strconv.FormatUint(uint64(evt.UserID), 10),
			BeforeState:  string(beforeState),
			AfterState:   string(afterState),
			IPAddress:    evt.IPAddress,
			TraceID:      evt.TraceID,
		})
	})

	return &UserProfileUpdatedSubscriber{}
}
