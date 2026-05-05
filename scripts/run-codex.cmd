@echo off
setlocal

powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0run-wizard-external.ps1"
