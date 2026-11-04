package controllers

import (
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

// userSelfResponse is only used after authenticating the account itself.
// The model hides CommonTags so public and managed-user responses stay private.
type userSelfResponse struct {
	models.User
	CommonTags []string `json:"commonTags"`
}

func selfUserResponse(user models.User) userSelfResponse {
	user.Password = ""
	return userSelfResponse{User: user, CommonTags: services.UserCommonTags(user.CommonTags)}
}
