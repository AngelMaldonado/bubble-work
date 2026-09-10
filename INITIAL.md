Ok, ahora vamos a platicar la V2 de este software y lo que se aprendió de V1. CLI y mirror de plane completamente innecesarios, Bubbles
puede evolucionar a un sistema completamente standalone y tener integraciones para canales de sincronización solo de cosas específicas.
Dicho esto dejame contarte mis objetivos con Bubbles:

## Objetivos y features
Primeramente, en mi organización y específicamente el departamento del cual estoy encargado (Software) hay 2 niveles:
  - Estratégico: es la capa superior en donde se analizan y organizan las actividades (threads), módulos (bubbles) y proyectos, es la capa de
    orquestación enfocada en objetivos que aporten valor a la organización.
  - Operativo: es la capa de ejecución y la que va a cubrir Bubble, los ejecutores usan inteligencia artificial para ejecutar velozmente las
    tareas, por lo que MCP será el protocolo principal de comunicación de los agentes IA de los operadores con Bubbles.

Enfocándonos en Bubbles, quiero que esta vez usemos Pocketbase, quiero ver qué cosas podemos usar que ofrezca este servicio selfhost para
evitar reinventar la rueda. El core que queremos conservar de V1 son:
- Bouyancy configurable de burbujas
- Estilo de burbujas, drawer de listado de threads, engine de visualización de markdown (MUST!), keybindings de navegación y omnibar
- Artefactos de documentación de proyectos

Requerimientos adicionales:
- Vista para planeador (líder - Capa estratégica): esta vista tendrá un inbox (tareas entrantes y vagas), calendario para visualizar las tareas con fecha de 
  entrega y modos de timeline - week - día. Y un Kanban de alto nivel que represente la organización enfocada a los objetivos. Esta vista tendrá también acceso
  a visualización y modificación de los objetivos, los cuales incialmente son:
  1. Clientes: el departamento de TI tiene un impacto muy grande en los clientes, ya que estos interactuan con distintos productos de software generados por
    la empresa, estos productos pueden aportar a:
      - Más ventas
      - Más retención
      - Más lealtad 
      - Mejor marketing
      - Más valor al cliente
  2. Rentabilidad de software: es la segunda prioridad, esto es muy importante para lograr que el departamento sea autosustentable, los sueldos e inversión
      económica en el departamento se justifique.
  3. Optimizar recursos: este se enfoca en aprovechar al máximo todos los recursos disponibles para el área e incluso para la empresa en general.
      - No tirar dinero a la basura
  4. ISO 9001: es el estándar de calidad que se maneja, todo enfocado a sistema documental, trazabilidad, y satisfacción al cliente. Nosotros tenemos ya
      implementado un sistema documental y trazabilidad por lo que se puede dar la libertad de ser flexibles y simplemente exponer la información para integración
      con sistemas externos.
  5. Mantenimiento: evitar deuda técnica para futuros problemas
- Sistema de prioridades:

| Prioridad | Significado | ¿Qué sucede? |
|-----------|-------------|--------------|
| P1 Crítica | Operación detenida | Interrumpe cualquier trabajo |
| P2 Alta | Impacto fuerte, pero hay workaround | Se atiende rápidamente |
| P3 Normal | Trabajo necesario normal | Entra en planeación |
| P4 Baja | Mejora, optimización nice-to-have | Backlog |

- Mapa determinante (solo para consulta o modificación):

| | Urgencia alta | Media | Baja |
|---|---|---|---|
| Impacto alto | P1 | P2 | P3 |
| Impacto medio | P2 | P2 | P3 |
| Impacto bajo | P3 | P3 | P4 |

-> Con esto respondemos: "Necesito esto urgentemente" -> Preguntamos: "¿Pasa algo si no se hace hoy?"
- MCP enfocado en edición de archivos markdown: los artefactos ya no serán guardados en base de datos de forma plain, es deseable mantener los archivos organizados
  en directorios y con sus extensiones .md, el MCP tendrá tools enfocadas en edición de estos archivos, mantener soporte para diagramas mermaid y agregar excalidraw.
  Usar como referencia este MCP: https://github.com/patrickomatik/mcp-file-edit
