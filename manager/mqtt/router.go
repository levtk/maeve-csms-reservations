package mqtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/santhosh-tekuri/jsonschema"
	"github.com/twlabs/ocpp2-broker-core/manager/handlers"
	handlers16 "github.com/twlabs/ocpp2-broker-core/manager/handlers/ocpp16"
	handlers201 "github.com/twlabs/ocpp2-broker-core/manager/handlers/ocpp201"
	"github.com/twlabs/ocpp2-broker-core/manager/ocpp"
	"github.com/twlabs/ocpp2-broker-core/manager/ocpp/ocpp16"
	"github.com/twlabs/ocpp2-broker-core/manager/ocpp/ocpp201"
	"github.com/twlabs/ocpp2-broker-core/manager/schemas"
	"github.com/twlabs/ocpp2-broker-core/manager/services"
	"io/fs"
	"k8s.io/utils/clock"
	"log"
	"reflect"
	"time"
)

type Router struct {
	CallRoutes       map[string]handlers.CallRoute
	CallResultRoutes map[string]handlers.CallResultRoute
}

func NewV16Router(emitter Emitter,
	clk clock.PassiveClock,
	tokenStore services.TokenStore,
	transactionStore services.TransactionStore,
	certValidationService services.CertificateValidationService,
	certSignerService services.CertificateSignerService,
	certProviderService services.EvCertificateProvider,
	heartbeatInterval time.Duration,
	schemaFS fs.FS) *Router {

	dataTransferCallMaker := DataTransferCallMaker{
		E: emitter,
		Actions: map[reflect.Type]DataTransferAction{
			reflect.TypeOf(&ocpp201.CertificateSignedRequestJson{}): {
				VendorId:  "org.openchargealliance.iso15118pnc",
				MessageId: "CertificateSigned",
			},
		},
	}

	return &Router{
		CallRoutes: map[string]handlers.CallRoute{
			"BootNotification": {
				NewRequest:     func() ocpp.Request { return new(ocpp16.BootNotificationJson) },
				RequestSchema:  "ocpp16/BootNotification.json",
				ResponseSchema: "ocpp16/BootNotificationResponse.json",
				Handler: handlers16.BootNotificationHandler{
					Clock:             clk,
					HeartbeatInterval: int(heartbeatInterval.Seconds()),
				},
			},
			"Heartbeat": {
				NewRequest:     func() ocpp.Request { return new(ocpp16.HeartbeatJson) },
				RequestSchema:  "ocpp16/Heartbeat.json",
				ResponseSchema: "ocpp16/HeartbeatResponse.json",
				Handler: handlers16.HeartbeatHandler{
					Clock: clk,
				},
			},
			"StatusNotification": {
				NewRequest:     func() ocpp.Request { return new(ocpp16.StatusNotificationJson) },
				RequestSchema:  "ocpp16/StatusNotification.json",
				ResponseSchema: "ocpp16/StatusNotificationResponse.json",
				Handler:        handlers.CallHandlerFunc(handlers16.StatusNotificationHandler),
			},
			"Authorize": {
				NewRequest:     func() ocpp.Request { return new(ocpp16.AuthorizeJson) },
				RequestSchema:  "ocpp16/Authorize.json",
				ResponseSchema: "ocpp16/AuthorizeResponse.json",
				Handler: handlers16.AuthorizeHandler{
					TokenStore: tokenStore,
				},
			},
			"StartTransaction": {
				NewRequest:     func() ocpp.Request { return new(ocpp16.StartTransactionJson) },
				RequestSchema:  "ocpp16/StartTransaction.json",
				ResponseSchema: "ocpp16/StartTransactionResponse.json",
				Handler: handlers16.StartTransactionHandler{
					Clock:            clk,
					TokenStore:       tokenStore,
					TransactionStore: transactionStore,
				},
			},
			"StopTransaction": {
				NewRequest:     func() ocpp.Request { return new(ocpp16.StopTransactionJson) },
				RequestSchema:  "ocpp16/StopTransaction.json",
				ResponseSchema: "ocpp16/StopTransactionResponse.json",
				Handler: handlers16.StopTransactionHandler{
					Clock:            clk,
					TransactionStore: transactionStore,
				},
			},
			"MeterValues": {
				NewRequest:     func() ocpp.Request { return new(ocpp16.MeterValuesJson) },
				RequestSchema:  "ocpp16/MeterValues.json",
				ResponseSchema: "ocpp16/MeterValuesResponse.json",
				Handler: handlers16.MeterValuesHandler{
					TransactionStore: transactionStore,
				},
			},
			"DataTransfer": {
				NewRequest:     func() ocpp.Request { return new(ocpp16.DataTransferJson) },
				RequestSchema:  "ocpp16/DataTransfer.json",
				ResponseSchema: "ocpp16/DataTransferResponse.json",
				Handler: handlers16.DataTransferHandler{
					SchemaFS: schemaFS,
					CallRoutes: map[string]map[string]handlers.CallRoute{
						"org.openchargealliance.iso15118pnc": {
							"Authorize": {
								NewRequest:     func() ocpp.Request { return new(ocpp201.AuthorizeRequestJson) },
								RequestSchema:  "ocpp201/AuthorizeRequest.json",
								ResponseSchema: "ocpp201/AuthorizeResponse.json",
								Handler: handlers201.AuthorizeHandler{
									TokenStore:                   tokenStore,
									CertificateValidationService: certValidationService,
								},
							},
							"GetCertificateStatus": {
								NewRequest:     func() ocpp.Request { return new(ocpp201.GetCertificateStatusRequestJson) },
								RequestSchema:  "ocpp201/GetCertificateStatusRequest.json",
								ResponseSchema: "ocpp201/GetCertificateStatusResponse.json",
								Handler: handlers201.GetCertificateStatusHandler{
									CertificateValidationService: certValidationService,
								},
							},
							"SignCertificate": {
								NewRequest:     func() ocpp.Request { return new(ocpp201.SignCertificateRequestJson) },
								RequestSchema:  "ocpp201/SignCertificateRequest.json",
								ResponseSchema: "ocpp201/SignCertificateResponse.json",
								Handler: handlers201.SignCertificateHandler{
									CertificateSignerService: certSignerService,
									CallMaker:                dataTransferCallMaker,
								},
							},
							"Get15118EVCertificate": {
								NewRequest:     func() ocpp.Request { return new(ocpp201.Get15118EVCertificateRequestJson) },
								RequestSchema:  "ocpp201/Get15118EVCertificateRequest.json",
								ResponseSchema: "ocpp201/Get15118EVCertificateResponse.json",
								Handler: handlers201.Get15118EvCertificateHandler{
									EvCertificateProvider: certProviderService,
								},
							},
						},
					},
				},
			},
		},
		CallResultRoutes: map[string]handlers.CallResultRoute{
			"DataTransfer": {
				NewRequest:  func() ocpp.Request { return new(ocpp16.DataTransferJson) },
				NewResponse: func() ocpp.Response { return new(ocpp16.DataTransferResponseJson) },
				Handler: handlers16.DataTransferResultHandler{
					SchemaFS: schemaFS,
					CallResultRoutes: map[string]map[string]handlers.CallResultRoute{
						"org.openchargealliance.iso15118pnc": {
							"CertificateSigned": {
								NewRequest:     func() ocpp.Request { return new(ocpp201.CertificateSignedRequestJson) },
								NewResponse:    func() ocpp.Response { return new(ocpp201.CertificateSignedResponseJson) },
								RequestSchema:  "ocpp201/CertificateSignedRequest.json",
								ResponseSchema: "ocpp201/CertificateSignedResponse.json",
								Handler:        handlers201.CertificateSignedResultHandler{},
							},
						},
					},
				},
			},
		},
	}
}

func NewV201Router(emitter Emitter,
	clk clock.PassiveClock,
	tokenStore services.TokenStore,
	transactionStore services.TransactionStore,
	tariffService services.TariffService,
	certValidationService services.CertificateValidationService,
	certSignerService services.CertificateSignerService,
	certProviderService services.EvCertificateProvider,
	heartbeatInterval time.Duration) *Router {

	callMaker := BasicCallMaker{
		E: emitter,
		Actions: map[reflect.Type]string{
			reflect.TypeOf(&ocpp201.CertificateSignedRequestJson{}): "CertificateSigned",
		},
	}

	return &Router{
		CallRoutes: map[string]handlers.CallRoute{
			"BootNotification": {
				NewRequest:     func() ocpp.Request { return new(ocpp201.BootNotificationRequestJson) },
				RequestSchema:  "ocpp201/BootNotificationRequest.json",
				ResponseSchema: "ocpp201/BootNotificationResponse.json",
				Handler: handlers201.BootNotificationHandler{
					Clock:             clk,
					HeartbeatInterval: int(heartbeatInterval.Seconds()),
				},
			},
			"Heartbeat": {
				NewRequest:     func() ocpp.Request { return new(ocpp201.HeartbeatRequestJson) },
				RequestSchema:  "ocpp201/HeartbeatRequest.json",
				ResponseSchema: "ocpp201/HeartbeatResponse.json",
				Handler: handlers201.HeartbeatHandler{
					Clock: clk,
				},
			},
			"StatusNotification": {
				NewRequest:     func() ocpp.Request { return new(ocpp201.StatusNotificationRequestJson) },
				RequestSchema:  "ocpp201/StatusNotificationRequest.json",
				ResponseSchema: "ocpp201/StatusNotificationResponse.json",
				Handler:        handlers.CallHandlerFunc(handlers201.StatusNotificationHandler),
			},
			"Authorize": {
				NewRequest:     func() ocpp.Request { return new(ocpp201.AuthorizeRequestJson) },
				RequestSchema:  "ocpp201/AuthorizeRequest.json",
				ResponseSchema: "ocpp201/AuthorizeResponse.json",
				Handler: handlers201.AuthorizeHandler{
					TokenStore:                   tokenStore,
					CertificateValidationService: certValidationService,
				},
			},
			"TransactionEvent": {
				NewRequest:     func() ocpp.Request { return new(ocpp201.TransactionEventRequestJson) },
				RequestSchema:  "ocpp201/TransactionEventRequest.json",
				ResponseSchema: "ocpp201/TransactionEventResponse.json",
				Handler: handlers201.TransactionEventHandler{
					TransactionStore: transactionStore,
					TariffService:    tariffService,
				},
			},
			"GetCertificateStatus": {
				NewRequest:     func() ocpp.Request { return new(ocpp201.GetCertificateStatusRequestJson) },
				RequestSchema:  "ocpp201/GetCertificateStatusRequest.json",
				ResponseSchema: "ocpp201/GetCertificateStatusResponse.json",
				Handler: handlers201.GetCertificateStatusHandler{
					CertificateValidationService: certValidationService,
				},
			},
			"SignCertificate": {
				NewRequest:     func() ocpp.Request { return new(ocpp201.SignCertificateRequestJson) },
				RequestSchema:  "ocpp201/SignCertificateRequest.json",
				ResponseSchema: "ocpp201/SignCertificateResponse.json",
				Handler: handlers201.SignCertificateHandler{
					CertificateSignerService: certSignerService,
					CallMaker:                callMaker,
				},
			},
			"Get15118EVCertificate": {
				NewRequest:     func() ocpp.Request { return new(ocpp201.Get15118EVCertificateRequestJson) },
				RequestSchema:  "ocpp201/Get15118EVCertificateRequest.json",
				ResponseSchema: "ocpp201/Get15118EVCertificateResponse.json",
				Handler: handlers201.Get15118EvCertificateHandler{
					EvCertificateProvider: certProviderService,
				},
			},
		},
		CallResultRoutes: map[string]handlers.CallResultRoute{
			"CertificateSigned": {
				NewRequest:     func() ocpp.Request { return new(ocpp201.CertificateSignedRequestJson) },
				NewResponse:    func() ocpp.Response { return new(ocpp201.CertificateSignedResponseJson) },
				RequestSchema:  "ocpp201/CertificateSignedRequest.json",
				ResponseSchema: "ocpp201/CertificateSignedResponse.json",
				Handler:        handlers201.CertificateSignedResultHandler{},
			},
		},
	}
}

func (r Router) Route(ctx context.Context, chargeStationId string, message Message, emitter Emitter, schemaFS fs.FS) error {
	switch message.MessageType {
	case MessageTypeCall:
		route, ok := r.CallRoutes[message.Action]
		if !ok {
			return fmt.Errorf("routing request: %w", NewError(ErrorNotImplemented, fmt.Errorf("%s not implemented", message.Action)))
		}
		err := schemas.Validate(message.RequestPayload, schemaFS, route.RequestSchema)
		if err != nil {
			var validationErr *jsonschema.ValidationError
			if errors.As(validationErr, &validationErr) {
				err = NewError(ErrorFormatViolation, err)
			}
			return fmt.Errorf("validating %s request: %w", message.Action, err)
		}
		req := route.NewRequest()
		err = json.Unmarshal(message.RequestPayload, &req)
		if err != nil {
			return fmt.Errorf("unmarshalling %s request payload: %w", message.Action, err)
		}
		resp, err := route.Handler.HandleCall(ctx, chargeStationId, req)
		if err != nil {
			return err
		}
		if resp == nil {
			return fmt.Errorf("no response or error for %s", message.Action)
		}
		responseJson, err := json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("marshalling %s call response: %w", message.Action, err)
		}
		err = schemas.Validate(responseJson, schemaFS, route.ResponseSchema)
		if err != nil {
			mqttErr := NewError(ErrorPropertyConstraintViolation, err)
			log.Printf("warning: response to %s is not valid: %v", message.Action, mqttErr)
		}
		out := &Message{
			MessageType:     MessageTypeCallResult,
			Action:          message.Action,
			MessageId:       message.MessageId,
			ResponsePayload: responseJson,
		}
		err = emitter.Emit(ctx, chargeStationId, out)
		if err != nil {
			return fmt.Errorf("sending call response: %w", err)
		}
	case MessageTypeCallResult:
		route, ok := r.CallResultRoutes[message.Action]
		if !ok {
			return fmt.Errorf("routing request: %w", NewError(ErrorNotImplemented, fmt.Errorf("%s result not implemented", message.Action)))
		}
		err := schemas.Validate(message.RequestPayload, schemaFS, route.RequestSchema)
		if err != nil {
			return fmt.Errorf("validating %s request: %w", message.Action, err)
		}
		err = schemas.Validate(message.ResponsePayload, schemaFS, route.ResponseSchema)
		if err != nil {
			var validationErr *jsonschema.ValidationError
			if errors.As(validationErr, &validationErr) {
				err = NewError(ErrorFormatViolation, err)
			}
			return fmt.Errorf("validating %s response: %w", message.Action, err)
		}
		req := route.NewRequest()
		err = json.Unmarshal(message.RequestPayload, &req)
		if err != nil {
			return fmt.Errorf("unmarshalling %s request payload: %w", message.Action, err)
		}
		resp := route.NewResponse()
		err = json.Unmarshal(message.ResponsePayload, &resp)
		if err != nil {
			return fmt.Errorf("unmarshalling %s response payload: %v", message.Action, err)
		}
		err = route.Handler.HandleCallResult(ctx, chargeStationId, req, resp, message.State)
		if err != nil {
			return err
		}
	case MessageTypeCallError:
		// TODO: what do we want to do with errors?
		return errors.New("we shouldn't get here at the moment")
	}

	return nil
}