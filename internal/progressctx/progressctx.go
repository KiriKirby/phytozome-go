// The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
// you may not use this file except in compliance with the License. You may obtain a copy of the License at
// https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
// basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
// Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
// wangsychn. All Rights Reserved. Contributor(s): .

package progressctx

import "context"

type contextKey struct{}

type UpdateFunc func(current int, message string)

func WithProgress(ctx context.Context, update UpdateFunc) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if update == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, update)
}

func FromContext(ctx context.Context) UpdateFunc {
	if ctx == nil {
		return nil
	}
	update, _ := ctx.Value(contextKey{}).(UpdateFunc)
	return update
}

func Report(ctx context.Context, current int, message string) {
	if update := FromContext(ctx); update != nil {
		update(current, message)
	}
}
