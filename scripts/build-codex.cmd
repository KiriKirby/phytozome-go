REM The contents of this file are subject to the Common Public Attribution License Version 1.0 (CPAL-1.0);
REM you may not use this file except in compliance with the License. You may obtain a copy of the License at
REM https://opensource.org/license/CPAL-1.0. Software distributed under the License is distributed on an "AS IS"
REM basis, WITHOUT WARRANTY OF ANY KIND, either express or implied. The Original Code is phytozome GO. The
REM Initial Developer is wangsychn. All portions of the code written by wangsychn are Copyright (c) 2026
REM wangsychn. All Rights Reserved. Contributor(s): .

@echo off
setlocal

powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0build-codex.ps1" %*
