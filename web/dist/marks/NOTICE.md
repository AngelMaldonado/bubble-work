# marks/

Logotipos de terceros, usados para enlazar a ese tercero.

- `github.png` — la marca de GitHub (el «Octocat»), render 3D aportado por el
  equipo. Recortada, con el borde comido un par de pixeles y suavizado para que
  no arrastre el halo blanco del fondo original, y reducida a 256px. Se usa
  únicamente para señalar enlaces a repositorios en GitHub. GitHub y el Octocat
  son marcas de GitHub, Inc.

Viven en `public/` y no en `src/assets/` porque se piden por URL desde el CSS y
desde `<img>`, y porque `public/logos/` —el otro directorio de marcas— se
GENERA y está ignorado: un archivo que la interfaz necesita siempre no puede
vivir donde `npm run logos` lo borra y lo rehace.
