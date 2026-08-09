package usecase

import (
	"fmt"
	"strconv"
	"strings"
)

func FormatPlatformAccountID(bindingID uint64, accountID string) string {
	return fmt.Sprintf("binding_%d_%s", bindingID, accountID)
}

func BindingIDFromPlatformAccountID(platformAccountID string) (uint64, error) {
	var bindingIDPart string
	switch {
	case strings.HasPrefix(platformAccountID, "binding_"):
		rest := strings.TrimPrefix(platformAccountID, "binding_")
		separator := strings.IndexByte(rest, '_')
		if separator <= 0 {
			return 0, fmt.Errorf("parse binding id from platform account id: invalid format %q", platformAccountID)
		}
		bindingIDPart = rest[:separator]
	case strings.HasPrefix(platformAccountID, "hoyo_ref_"):
		rest := strings.TrimPrefix(platformAccountID, "hoyo_ref_")
		separator := strings.IndexByte(rest, '_')
		if separator <= 0 {
			return 0, fmt.Errorf("parse binding id from platform account id: invalid format %q", platformAccountID)
		}
		bindingIDPart = rest[:separator]
	default:
		return 0, fmt.Errorf("parse binding id from platform account id: invalid format %q", platformAccountID)
	}

	bindingID, err := strconv.ParseUint(bindingIDPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse binding id from platform account id: %w", err)
	}

	return bindingID, nil
}
