// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package tui

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSearchDebounceDelay = 35 * time.Millisecond
	defaultUIThrottleDelay     = 80 * time.Millisecond
	defaultUIAnimationDelay    = 120 * time.Millisecond
)

func searchDebounceDelay() time.Duration {
	return configuredDurationMS("PHYTOZOME_GO_SEARCH_DEBOUNCE_MS", defaultSearchDebounceDelay)
}

func uiThrottleDelay() time.Duration {
	return configuredDurationMS("PHYTOZOME_GO_UI_THROTTLE_MS", defaultUIThrottleDelay)
}

func uiAnimationDelay() time.Duration {
	return configuredDurationMS("PHYTOZOME_GO_UI_ANIMATION_MS", defaultUIAnimationDelay)
}

func configuredDurationMS(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return time.Duration(parsed) * time.Millisecond
}
