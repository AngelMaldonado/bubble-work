<script lang="ts">
  // Una fecha de entrega, con el calendario de Skeleton.
  //
  // Un componente y no una copia: el `DatePicker` de Skeleton se arma pieza a
  // pieza —control, disparador, posicionador, tabla, cabecera, celdas— y son
  // cincuenta líneas de marcado. Dos copias de eso divergen a la primera
  // corrección, y la que se quede atrás es la que nadie mira.
  //
  // Un `<input type="date">` nativo no sirve aquí por lo mismo que un `<select>`
  // no servía en los filtros: su calendario lo dibuja el sistema operativo,
  // donde no llega ninguna hoja de estilo.
  //
  // El valor va y viene como `2026-12-24`. Vacío es «sin fecha», que es una
  // respuesta y no un hueco.
  import { DatePicker, parseDate } from '@skeletonlabs/skeleton-svelte';

  let {
    value = '',
    label = '',
    placeholder = 'sin fecha',
    onchange,
  }: {
    /** `2026-12-24`, o vacío */
    value?: string;
    label?: string;
    placeholder?: string;
    onchange?: (day: string) => void;
  } = $props();

  const picked = $derived.by(() => {
    if (!value) return [];
    try {
      return [parseDate(value.slice(0, 10))];
    } catch {
      // Un valor que no es una fecha —datos viejos, un texto libre— enseña el
      // calendario vacío en vez de romper la pantalla que lo contiene.
      return [];
    }
  });

  /** El día elegido, como `2026-12-24`.
   *
   *  NO `valueAsString`: Zag lo formatea para el LOCALE, así que con `es-MX`
   *  devuelve `24/12/2026`, el servidor lo rechaza y el campo vuelve a «sin
   *  fecha» al recargar. Una fecha con tres números es la misma en cualquier
   *  idioma. */
  function isoDate(d?: { year: number; month: number; day: number }) {
    if (!d) return '';
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${d.year}-${pad(d.month)}-${pad(d.day)}`;
  }
</script>

<DatePicker
  openOnClick
  value={picked}
  onValueChange={(e: { value: { year: number; month: number; day: number }[] }) =>
    onchange?.(isoDate(e.value?.[0]))}
  locale="es-MX"
  startOfWeek={1}>
  {#if label}<DatePicker.Label class="cap">{label}</DatePicker.Label>{/if}
  <DatePicker.Control class="dp-control">
    <DatePicker.Input {placeholder} autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
    <DatePicker.Trigger>🗓</DatePicker.Trigger>
  </DatePicker.Control>
  <!-- SIN portal, al revés que el resto de los popups. Un diálogo modal apaga
       los eventos de puntero fuera de sí y los devuelve capa a capa; un
       calendario que aterriza en el `body` está fuera, así que se dibujaba bien
       e ignoraba cada clic. Dentro del diálogo es parte de la capa activa, y
       Zag lo posiciona `fixed` de todos modos, así que nada lo recorta. -->
  <DatePicker.Positioner>
    <DatePicker.Content class="dp-content">
      <DatePicker.View view="day">
        <DatePicker.Context>
          {#snippet children(dp)}
            <DatePicker.ViewControl class="dp-nav">
              <DatePicker.PrevTrigger>‹</DatePicker.PrevTrigger>
              <DatePicker.ViewTrigger>
                <DatePicker.RangeText />
              </DatePicker.ViewTrigger>
              <DatePicker.NextTrigger>›</DatePicker.NextTrigger>
            </DatePicker.ViewControl>
            <DatePicker.Table>
              <DatePicker.TableHead>
                <DatePicker.TableRow>
                  {#each dp().weekDays as d, i (i)}
                    <DatePicker.TableHeader>{d.short}</DatePicker.TableHeader>
                  {/each}
                </DatePicker.TableRow>
              </DatePicker.TableHead>
              <DatePicker.TableBody>
                {#each dp().weeks as week, i (i)}
                  <DatePicker.TableRow>
                    {#each week as day, j (j)}
                      <DatePicker.TableCell value={day}>
                        <DatePicker.TableCellTrigger>{day.day}</DatePicker.TableCellTrigger>
                      </DatePicker.TableCell>
                    {/each}
                  </DatePicker.TableRow>
                {/each}
              </DatePicker.TableBody>
            </DatePicker.Table>
          {/snippet}
        </DatePicker.Context>
      </DatePicker.View>
    </DatePicker.Content>
  </DatePicker.Positioner>
</DatePicker>
