/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */

package util

import (
	"time"
)

// FinishedResult returns a new SyncResult that call IsFinished() on it will return true
func FinishedResult() *SyncResult {
	return &SyncResult{}
}

// ErrorResult returns a new SyncResult that call IsError() on it will return true
func ErrorResult(err error) *SyncResult {
	return &SyncResult{
		faild: &failedOp{
			reason: err.Error(),
		},
	}
}

// FailResult returns a new SyncResult that call IsFailed() on it will return true
func FailResult(delay time.Duration, msg string) *SyncResult {
	return &SyncResult{
		faild: &failedOp{
			reason:         msg,
			nextRetryDelay: delay,
			isWebhookFail:  true,
		},
	}
}

// AsyncResult returns a new SyncResult that call IsPeriodic() on it will return true
func AsyncResult(period time.Duration) *SyncResult {
	return &SyncResult{
		async: &asyncOp{
			nextCheckDelay: period,
		},
	}
}

// PeriodicResult returns a new SyncResult that call IsPeriodic() on it will return true
func PeriodicResult(period time.Duration) *SyncResult {
	return &SyncResult{
		periodic: &periodicOp{
			nextRunDelay: period,
		},
	}
}

// SyncResult stores result for sync method of controllers
type SyncResult struct {
	faild    *failedOp
	async    *asyncOp
	periodic *periodicOp
}

// IsFinished indicates the operation is successfully finished
func (s *SyncResult) IsFinished() bool {
	return !s.IsFailed() && !s.IsRunning() && !s.IsPeriodic()
}

// IsFailed indicates no error occured during operation, but the operation failed
func (s *SyncResult) IsFailed() bool {
	return s.faild != nil
}

// IsRunning indicates the operation is still in progress
func (s *SyncResult) IsRunning() bool {
	return s.async != nil
}

// IsPeriodic indicates the operation successfully finished and should be called periodically
func (s *SyncResult) IsPeriodic() bool {
	return s.periodic != nil
}

// GetFailReason returns the error stored in SyncResult
func (s *SyncResult) GetFailReason() string {
	if s.faild == nil {
		return ""
	}
	return s.faild.reason
}

// GetNextRun returns in how long time the operation should be retried
func (s *SyncResult) GetNextRun() time.Duration {
	if s.faild != nil {
		return s.faild.nextRetryDelay
	} else if s.async != nil {
		return s.async.nextCheckDelay
	} else if s.periodic != nil {
		return s.periodic.nextRunDelay
	}
	return 0
}

type failedOp struct {
	nextRetryDelay time.Duration
	reason         string
	isWebhookFail  bool
}

type asyncOp struct {
	nextCheckDelay time.Duration
}

type periodicOp struct {
	nextRunDelay time.Duration
}
