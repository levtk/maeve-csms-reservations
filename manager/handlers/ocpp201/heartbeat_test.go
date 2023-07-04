package ocpp201_test

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	handlers "github.com/twlabs/ocpp2-broker-core/manager/handlers/ocpp201"
	types "github.com/twlabs/ocpp2-broker-core/manager/ocpp/ocpp201"
	clockTest "k8s.io/utils/clock/testing"
	"testing"
	"time"
)

func TestHeartbeatHandler(t *testing.T) {
	now, err := time.Parse(time.RFC3339, "2023-06-15T15:05:00+01:00")
	require.NoError(t, err)
	clock := clockTest.NewFakePassiveClock(now)

	handler := handlers.HeartbeatHandler{
		Clock: clock,
	}

	req := &types.HeartbeatRequestJson{}

	got, err := handler.HandleCall(context.Background(), "cs001", req)
	assert.NoError(t, err)

	want := &types.HeartbeatResponseJson{
		CurrentTime: "2023-06-15T15:05:00+01:00",
	}

	assert.Equal(t, want, got)
}