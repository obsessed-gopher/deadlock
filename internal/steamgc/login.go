package steamgc

import (
	"context"
	"fmt"

	steam "github.com/paralin/go-steam"
	"github.com/paralin/go-steam/protocol/protobuf/unified"
)

// Guard asks the person at the keyboard for a Steam Guard step. code is
// empty when the step is an approval in the Steam app or by email link.
type Guard func(ctx context.Context, kind GuardKind, hint string) (code string, err error)

// GuardKind is the Steam Guard step Steam asked for.
type GuardKind int

// The Steam Guard steps, in the order Steam may offer them.
const (
	GuardEmailCode   GuardKind = iota // a code sent by email
	GuardDeviceCode                   // a code from the Steam mobile authenticator
	GuardAppApproval                  // approve the login in the Steam app
	GuardEmailLink                    // approve the login via a link in the email
)

// Login exchanges the bot's credentials for a long-lived refresh token, going
// through Steam Guard with guard. The password is used once and not kept.
func Login(ctx context.Context, username, password string, guard Guard) (string, error) {
	sc := steam.NewClient()
	token, err := sc.Auth.Authenticate(ctx, &steam.LogOnDetails{
		Username:               username,
		Password:               password,
		DeviceFriendlyName:     "deadlock stats",
		ShouldRememberPassword: true,
		ConfirmSteamGuard: func(ctx context.Context, cs []steam.SteamGuardConfirmation) (string, unified.EAuthSessionGuardType, error) {
			// Prefer what needs no typing, then the authenticator, then email.
			order := []struct {
				t    unified.EAuthSessionGuardType
				kind GuardKind
			}{
				{unified.EAuthSessionGuardType_k_EAuthSessionGuardType_DeviceConfirmation, GuardAppApproval},
				{unified.EAuthSessionGuardType_k_EAuthSessionGuardType_DeviceCode, GuardDeviceCode},
				{unified.EAuthSessionGuardType_k_EAuthSessionGuardType_EmailConfirmation, GuardEmailLink},
				{unified.EAuthSessionGuardType_k_EAuthSessionGuardType_EmailCode, GuardEmailCode},
			}
			for _, o := range order {
				for _, c := range cs {
					if c.Type != o.t {
						continue
					}
					code, err := guard(ctx, o.kind, c.Message)
					return code, o.t, err
				}
			}
			return "", unified.EAuthSessionGuardType_k_EAuthSessionGuardType_Unknown, fmt.Errorf("steam gc: unsupported steam guard: %v", cs)
		},
	})
	if err != nil {
		return "", fmt.Errorf("steam gc: login: %w", err)
	}
	return token, nil
}
