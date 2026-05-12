package models

import "time"

type UserSMTPServer struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Name        string    `json:"name"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Username    string    `json:"username"`
	PasswordEnc string    `json:"-"`
	Password    string    `json:"-"`
	Encryption  string    `json:"encryption"`
	FromAddress string    `json:"from_address"`
	FromName    string    `json:"from_name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
