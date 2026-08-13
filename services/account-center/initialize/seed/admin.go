package seed

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/secretfile"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"paigram/internal/model"
)

// defaultAdminEmail is used when ADMIN_EMAIL is not provided.
const defaultAdminEmail = "admin@paigram.local"

// DefaultBcryptCost matches the OWASP-recommended cost the rest of the
// system uses when an explicit Config is unavailable. Callers that have
// access to *config.Config should pass cfg.GetBcryptCost() instead.
const DefaultBcryptCost = 12

// AdminConfig holds configuration for creating the default admin user.
type AdminConfig struct {
	Email       string
	Password    string
	DisplayName string
}

// resolveAdminConfig builds the admin config from an external secret file or
// environment variables.
//
// A supplied password is mandatory. We deliberately refuse to auto-generate
// one because logging or writing the generated value would leak it, while
// printing only to a TTY breaks unattended deployment. Failing closed makes
// the operator choose the value without exposing it through process arguments.
func resolveAdminConfig() (AdminConfig, error) {
	email := os.Getenv("ADMIN_EMAIL")
	if email == "" {
		email = defaultAdminEmail
	}

	password := os.Getenv("ADMIN_PASSWORD")
	if passwordFile := os.Getenv("ADMIN_PASSWORD_FILE"); passwordFile != "" {
		loadedPassword, err := secretfile.Read(passwordFile)
		if err != nil {
			return AdminConfig{}, fmt.Errorf("ADMIN_PASSWORD_FILE: %w", err)
		}
		password = loadedPassword
	}
	if password == "" {
		return AdminConfig{}, errors.New(
			"ADMIN_PASSWORD_FILE or ADMIN_PASSWORD must be set; refusing to auto-generate a password " +
				"(would leak via logs or files). Provide a strong " +
				"value (>=8 chars) and re-run seed.")
	}

	displayName := os.Getenv("ADMIN_NAME")
	if displayName == "" {
		displayName = "Administrator"
	}

	return AdminConfig{
		Email:       email,
		Password:    password,
		DisplayName: displayName,
	}, nil
}

// CreateDefaultAdmin creates a default admin user if it doesn't exist.
//
// Credentials come from an external secret file or environment variables:
//
//	ADMIN_EMAIL    - optional, defaults to admin@paigram.local
//	ADMIN_PASSWORD_FILE - preferred password secret file
//	ADMIN_PASSWORD      - fallback; the call fails closed if both are unset
//	ADMIN_NAME     - optional, defaults to "Administrator"
//
// bcryptCost should be the operator-configured cost (typically
// cfg.GetBcryptCost()). A value below 10 is bumped to DefaultBcryptCost.
func CreateDefaultAdmin(db *gorm.DB, bcryptCost int) error {
	// Check if admin user already exists
	var adminRole model.Role
	if err := db.Where("name = ?", model.RoleAdmin).First(&adminRole).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("admin role not found, please run seed roles first")
		}
		return fmt.Errorf("check admin role: %w", err)
	}

	administratorExists, err := recoveryAdministratorExists(db)
	if err != nil {
		return fmt.Errorf("check existing admins: %w", err)
	}
	if administratorExists {
		log.Printf("Admin user already exists, skipping creation")
		return nil
	}

	cfg, err := resolveAdminConfig()
	if err != nil {
		return err
	}

	log.Printf("Creating default admin user with email: %s", cfg.Email)

	if err := createAdminUser(db, cfg, adminRole.ID, bcryptCost); err != nil {
		return err
	}

	log.Println("================================================================")
	log.Println("  Default admin user has been bootstrapped.")
	log.Printf("  Email: %s", cfg.Email)
	log.Println("  Password: (loaded from configured bootstrap secret)")
	log.Println("  Rotate the password via the admin UI as soon as possible.")
	log.Println("================================================================")
	return nil
}

func recoveryAdministratorExists(db *gorm.DB) (bool, error) {
	var exists bool
	err := db.Raw("SELECT recovery_administrator_exists()").Scan(&exists).Error
	return exists, err
}

// resolveBcryptCost clamps the cost into the safe [10,14] range.
func resolveBcryptCost(cost int) int {
	if cost < 10 {
		return DefaultBcryptCost
	}
	if cost > 14 {
		return 14
	}
	return cost
}

// createAdminUser creates an admin user with the given configuration.
func createAdminUser(db *gorm.DB, config AdminConfig, adminRoleID uint64, bcryptCost int) error {
	// Hash password with the operator-configured cost (V8).
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(config.Password), resolveBcryptCost(bcryptCost))
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	// Start transaction
	return db.Transaction(func(tx *gorm.DB) error {
		// Create user
		user := model.User{
			PrimaryLoginType: model.LoginTypeEmail,
			Status:           model.UserStatusActive,
		}

		if err := tx.Create(&user).Error; err != nil {
			return fmt.Errorf("create user: %w", err)
		}

		// Create user profile
		profile := model.UserProfile{
			UserID:      user.ID,
			DisplayName: config.DisplayName,
			Locale:      "en_US",
		}

		if err := tx.Create(&profile).Error; err != nil {
			return fmt.Errorf("create profile: %w", err)
		}

		// Create user email
		email := model.UserEmail{
			UserID:    user.ID,
			Email:     config.Email,
			IsPrimary: true,
			VerifiedAt: sql.NullTime{
				Time:  time.Now(),
				Valid: true,
			},
		}

		if err := tx.Create(&email).Error; err != nil {
			return fmt.Errorf("create email: %w", err)
		}

		// Create user credential
		credential := model.UserCredential{
			UserID:            user.ID,
			Provider:          "email",
			ProviderAccountID: config.Email,
			PasswordHash:      string(passwordHash),
		}

		if err := tx.Create(&credential).Error; err != nil {
			return fmt.Errorf("create credential: %w", err)
		}

		// Assign admin role
		userRole := model.UserRole{
			UserID:    user.ID,
			RoleID:    adminRoleID,
			GrantedBy: user.ID, // Self-granted for initial admin
		}

		if err := tx.Create(&userRole).Error; err != nil {
			return fmt.Errorf("assign admin role: %w", err)
		}

		log.Printf("Successfully created admin user (ID: %d) with email: %s", user.ID, config.Email)
		log.Printf("IMPORTANT: Please change the default admin password immediately!")

		return nil
	})
}
