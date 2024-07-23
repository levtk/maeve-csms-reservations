package ocpp16

type ReservationResponse struct {
	Status ReservationStatus `json:"status" yaml:"status" mapstructure:"status"`
}

type ReservationStatus string

const ReservationStatusAccepted ReservationStatus = "Accepted"
const ReservationStatusFaulted ReservationStatus = "Faulted"
const ReservationStatusOccupied ReservationStatus = "Occupied"
const ReservationStatusRejected ReservationStatus = "Rejected"
const ReservationStatusUnavailable ReservationStatus = "Unavailable"

func (*ReservationResponse) IsResponse() {}
