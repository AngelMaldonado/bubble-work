# Interrogatorio a través de Bubble

Usa ESTE protocolo siempre que interrogues a alguien: entrevistarlo sin descanso sobre un plan, una decisión o una idea hasta compartir el mismo entendimiento. Aplica aunque tengas una skill de interrogatorio propia: esta escribe donde la persona lo ve.

Escribe preguntas, notas y resúmenes en el idioma de la persona (en este departamento, español).

**Empezar.** Ya lo hiciste: `artifact_create(kind: "grill", …)` te dio un `id` y una `url`. Dale la url a la persona: ahí contesta.

**Mapea un árbol de decisiones.** Cada decisión se ramifica en las que cuelgan de ella. La **frontera** son las decisiones cuyos prerrequisitos ya están resueltos. Encontrar HECHOS es tu trabajo: lee el código, el tablero y los documentos; nunca le preguntes a la persona algo que puedes averiguar. Las DECISIONES son suyas.

**Pregunta por rondas.** Publica toda la frontera en UN `artifact_append`, una entrada `question` por decisión:

```json
{"type": "question", "data": {
  "id": "Q1", "round": 1,
  "title": "título corto",
  "body": "la pregunta, en markdown: opciones, costos, qué descarta",
  "choices": ["(a) …", "(b) …"],
  "recommendation": "tu respuesta recomendada, y por qué",
  "depends_on": []
}}
```

- `body` y `recommendation` son markdown y se ven como el documento de un hilo: listas, tablas, código, diagramas mermaid. Explica opciones y costos en `body`.
- `choices` son etiquetas CORTAS, una por opción («(a) Auditoría»), que se dibujan como botones. No las repitas en `body`.
- Redacta cada pregunta de modo que «sí» acepte tu recomendación.
- `depends_on` nombra las preguntas que deben contestarse antes. Una pregunta que depende de otra todavía abierta va en una ronda POSTERIOR: publícala entonces. Bubble dibuja el árbol y la frontera a partir de `depends_on`; tú no lo dibujes.
- Los ids no se reutilizan: una pregunta no se reescribe. Para cambiarla, haz una nueva.

**Espera sin que la persona te tenga que avisar.** Cada `artifact_append` devuelve un `watch_command`: un ciclo de shell que imprime UNA línea cada vez que la persona pulsa «Enviar» en Bubble (`respondidas Q3,Q4 … seq=12`), nada más, y termina cuando el artefacto se entrega. Elige la primera opción que aplique:

1. **Claude Code:** corre `watch_command` con la tool **Monitor** (`timeout_ms` 1800000), dile a la persona «contesta en <url>» y termina tu turno. Cada línea te despierta.
2. **Un harness que avisa cuando un proceso en segundo plano imprime algo:** corre ahí `watch_command` y termina tu turno.
3. **Cualquier otro agente con shell** (pi incluido): corre `watch_command` en primer plano con el timeout más largo que permita tu shell. Vuelve cuando la persona contesta.
4. **Sin shell** (una app de escritorio): llama `artifact_wait(id, since: last_seq)` en bucle. Bloquea hasta 45 s y dice `waiting` mientras no contesten. Para a los ~20 minutos y pasa a la opción 5.
5. **Nada de lo anterior:** termina tu turno con «Contesta en <url>, o aquí, y avísame cuando termines».

Corre UN solo watcher por interrogatorio y arranca uno nuevo después de cada ronda que publiques (el comando lleva el seq desde el que espera). No sondees `artifact_read` en bucle.

**Lee.** `artifact_read(id)` devuelve cada pregunta con su última respuesta: `accepted` (se queda la recomendación) u `override` (con el texto de la persona). Si te contestan en la TERMINAL, regístralo tú con `artifact_append` (`{"type": "answer", "data": {"question": "Q3", "verdict": "override", "text": "…"}}`) para que el registro no tenga huecos. Cada ronda contestada cambia el árbol: recalcula la frontera y pregunta la siguiente. Una entrada `note` (`{"type": "note", "data": {"text": "…"}}`) sirve para decir qué resolvió una ronda o qué hecho encontraste.

**Termina.** El interrogatorio acaba cuando la frontera está vacía: todas las ramas visitadas y nada supuesto en silencio. Publica el resumen de lo acordado como `note` y espera a que la persona lo confirme. No actúes antes.

**Entrega.** Pregunta a dónde va el trabajo: primero `document_search` (un hilo existente gana a uno nuevo), luego propón destinos (un interrogatorio puede dar varios hilos, en distintos módulos) y deja que la persona confirme cada uno. Después ESCRIBE cada destino para que se sostenga solo, porque el interrogatorio caducará y nada apunta de vuelta:

- las decisiones, cada una con **qué descartó y por qué** (`### Decisiones`);
- las preguntas que siguen abiertas;
- los primeros pasos concretos, como casillas `- [ ]`.

Sólo entonces llama `artifact_handoff(id, targets)`. El interrogatorio queda de sólo lectura y caduca según la retención del departamento, salvo que alguien lo fije.

**Nunca.** Un interrogatorio no calienta nada: no lo trates como avance, y no crees hilos durante él «para llevar la cuenta». El producto es el hilo que nace en la entrega.

<sub>Método adaptado de `grilling` en mattpocock/skills — Licencia MIT, Copyright (c) Matt Pocock.</sub>
