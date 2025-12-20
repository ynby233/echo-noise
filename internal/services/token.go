package services

import (
    "fmt"
    "github.com/rcy1314/echo-noise/internal/models"
    "github.com/rcy1314/echo-noise/internal/repository"
)

func GetUserToken(userID uint) (string, error) {
    user, err := repository.GetUserByID(userID)
    if err != nil {
        return "", fmt.Errorf("获取用户信息失败: %v", err)
    }

    if user.Token == "" {
        token, err := RegenerateUserToken(userID)
        if err != nil {
            return "", fmt.Errorf("生成用户 token 失败: %v", err)
        }
        return token, nil
    }
    return user.Token, nil
}

func RegenerateUserToken(userID uint) (string, error) {
    user, err := repository.GetUserByID(userID)
    if err != nil {
        return "", fmt.Errorf("用户不存在: %v", err)
    }

    newToken := models.GenerateToken(32)
    
    if err := repository.UpdateUserToken(user.ID, newToken); err != nil {
        return "", fmt.Errorf("更新用户 token 失败: %v", err)
    }
    
    return newToken, nil
}