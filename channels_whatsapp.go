//go:build !nowhatsapp

package main

// El canal de WhatsApp, compilado en el binario. Con `-tags nowhatsapp` se deja
// fuera: sin él no hay dependencia de whatsmeow ni sesión que guardar.
import _ "github.com/AngelMaldonado/bubble-work/internal/channels/whatsapp"
