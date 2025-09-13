package scraper

import "laundry-status-backend/internal/store"

// ApiResponse models the top-level structure of the upstream API's response.
type ApiResponse struct {
	Code int `json:"code"`
	Data struct {
		Page     int             `json:"page"`
		PageSize int             `json:"pageSize"`
		Total    int             `json:"total"`
		Items    []store.ApiItem `json:"items"`
	} `json:"data"`
}
