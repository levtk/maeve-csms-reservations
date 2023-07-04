package ocpp16

import (
	"context"
	"github.com/twlabs/ocpp2-broker-core/manager/ocpp"
	types "github.com/twlabs/ocpp2-broker-core/manager/ocpp/ocpp16"
	"github.com/twlabs/ocpp2-broker-core/manager/services"
	"log"
)

type AuthorizeHandler struct {
	TokenStore services.TokenStore
}

func (a AuthorizeHandler) HandleCall(ctx context.Context, chargeStationId string, request ocpp.Request) (ocpp.Response, error) {
	req := request.(*types.AuthorizeJson)
	log.Printf("Charge station %s authorize token %s", chargeStationId, req.IdTag)

	status := types.AuthorizeResponseJsonIdTagInfoStatusInvalid
	tok, err := a.TokenStore.FindToken("ISO14443", req.IdTag)
	if err != nil {
		return nil, err
	}
	if tok != nil {
		status = types.AuthorizeResponseJsonIdTagInfoStatusAccepted
	}

	return &types.AuthorizeResponseJson{
		IdTagInfo: types.AuthorizeResponseJsonIdTagInfo{
			Status: status,
		},
	}, nil
}