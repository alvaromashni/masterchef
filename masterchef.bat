@echo off
rem Duplo clique abre o masterchef. Para um botao: botao direito > Enviar para > Area de trabalho.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0masterchef.ps1"
if errorlevel 1 pause
