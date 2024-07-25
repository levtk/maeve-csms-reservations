package ocpp16

import (
	"context"

	"github.com/thoughtworks/maeve-csms/manager/ocpp"
	"github.com/thoughtworks/maeve-csms/manager/ocpp/ocpp16"
	"golang.org/x/exp/slog"
)

type ReservationResultHandler struct{}

func (r ReservationResultHandler) HandleCallResult(ctx context.Context, chargeStationId string, request ocpp.Request, response ocpp.Response, state any) error {
	req := request.(*ocpp16.Reservation)
	resp := response.(*ocpp16.ReservationResponse)

	slog.Info("Handling Result for reservation: ", slog.Any("request", req), slog.Any("response", resp))

	return nil
}
