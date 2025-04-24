package web

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// ParamInt parses a route parameter as int
func ParamInt(c *fiber.Ctx, key string) (int, error) {
	return strconv.Atoi(c.Params(key))
}

// respond sends a JSON response with status code
func respond(c *fiber.Ctx, status int, body interface{}) error {
	return c.Status(status).JSON(body)
}

// respondError sends an error JSON response
func respondError(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(fiber.Map{"error": message})
}
