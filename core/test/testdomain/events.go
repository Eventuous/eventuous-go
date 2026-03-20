package testdomain

type RoomBooked struct {
	BookingID string  `json:"bookingId"`
	RoomID    string  `json:"roomId"`
	CheckIn   string  `json:"checkIn"`
	CheckOut  string  `json:"checkOut"`
	Price     float64 `json:"price"`
}

type BookingImported struct {
	BookingID string  `json:"bookingId"`
	RoomID    string  `json:"roomId"`
	Price     float64 `json:"price"`
}

type PaymentRecorded struct {
	BookingID string  `json:"bookingId"`
	Amount    float64 `json:"amount"`
}

type BookingCancelled struct {
	BookingID string `json:"bookingId"`
	Reason    string `json:"reason"`
}
