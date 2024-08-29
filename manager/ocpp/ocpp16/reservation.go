package ocpp16

type Reservation struct {
	ReservationId int    `json:"reservationId" yaml:"reservationId" mapstructure:"reservationId"`
	ConnectorId   int    `json:"connectorId,omitempty" yaml:"connectorId,omitempty" mapstructure:"connectorId,omitempty"`
	ExpiryDate    string `json:"expiryDate" yaml:"expiryDate" mapstructure:"expiryDate"`
	IdTag         string `json:"idTag" yaml:"idTag" mapstructure:"idTag"`
}

func (*Reservation) IsRequest() {}
