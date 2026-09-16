<script lang="ts">
  import { onMount } from 'svelte';
  import { translate, translateMessage, type Locale } from '$lib/i18n';
  import type { Map as MapboxMap, Popup, GeoJSONSource } from 'mapbox-gl';
  import 'mapbox-gl/dist/mapbox-gl.css';
  import { needsReview } from '$lib/export';
  import { pointOf } from '$lib/geo';
  import type { Business } from '$lib/types';

  let {
    rows,
    locale,
    token,
    onopen
  }: {
    locale: Locale;
    rows: Business[];
    token: string;
    onopen: (row: Business) => void;
  } = $props();

  const t = (text: string, values?: Record<string, string | number>) =>
    translate(locale, text, values);
  const number = (value: number) =>
    value.toLocaleString(locale === 'nl' ? 'nl-BE' : 'en-GB');
  const SOURCE = 'businesses';
  const LAYER = 'business-points';
  const COLOURS = { reviewed: '#4b7435', review: '#b7852f', ok: '#7c917a' };

  let container = $state<HTMLDivElement>();
  let map: MapboxMap | undefined;
  let popup: Popup | undefined;
  let ready = $state(false);
  let failed = $state('');

  function statusOf(row: Business): keyof typeof COLOURS {
    if (row.reviewed) return 'reviewed';
    if (needsReview(row)) return 'review';
    return 'ok';
  }

  function collection(list: Business[]): GeoJSON.FeatureCollection {
    const features: GeoJSON.Feature[] = [];
    for (const row of list) {
      const point = pointOf(row.geometry);
      if (!point) continue;
      features.push({
        type: 'Feature',
        geometry: { type: 'Point', coordinates: point },
        properties: {
          id: row.id,
          name: row.name || t('Unnamed business'),
          address: row.address || t('No address provided'),
          status: statusOf(row)
        }
      });
    }
    return { type: 'FeatureCollection', features };
  }

  let plotted = $derived(collection(rows));
  let missing = $derived(rows.length - plotted.features.length);

  function fit(data: GeoJSON.FeatureCollection) {
    if (!map || !data.features.length) return;
    let west = 180,
      south = 90,
      east = -180,
      north = -90;
    for (const feature of data.features) {
      const [lng, lat] = (feature.geometry as GeoJSON.Point).coordinates;
      west = Math.min(west, lng);
      east = Math.max(east, lng);
      south = Math.min(south, lat);
      north = Math.max(north, lat);
    }
    map.fitBounds(
      [
        [west, south],
        [east, north]
      ],
      { padding: 48, maxZoom: 15, duration: 400 }
    );
  }

  onMount(() => {
    let disposed = false;
    (async () => {
      try {
        const mapboxgl = (await import('mapbox-gl')).default;
        if (disposed || !container) return;
        mapboxgl.accessToken = token;
        map = new mapboxgl.Map({
          container,
          style: 'mapbox://styles/mapbox/light-v11',
          center: [4.4, 50.85],
          zoom: 7,
          attributionControl: true,
          cooperativeGestures: true,
          locale: {
            'NavigationControl.ZoomIn': t('Zoom in'),
            'NavigationControl.ZoomOut': t('Zoom out'),
            'NavigationControl.ResetBearing': t('Reset bearing to north'),
            'TouchPanBlocker.Message': t('Use two fingers to move the map'),
            'ScrollZoomBlocker.CtrlMessage': t(
              'Use Ctrl + scroll to zoom the map'
            ),
            'ScrollZoomBlocker.CmdMessage': t('Use ⌘ + scroll to zoom the map')
          }
        });
        map.addControl(new mapboxgl.NavigationControl(), 'top-right');
        popup = new mapboxgl.Popup({
          closeButton: false,
          closeOnClick: false,
          offset: 10,
          maxWidth: '260px'
        });
        map.on('error', (event) => {
          if (!ready)
            failed =
              event.error?.message ||
              'The map could not be loaded. Check the Mapbox token.';
        });
        map.on('load', () => {
          if (!map) return;
          map.addSource(SOURCE, { type: 'geojson', data: plotted });
          map.addLayer({
            id: LAYER,
            type: 'circle',
            source: SOURCE,
            paint: {
              'circle-radius': [
                'interpolate',
                ['linear'],
                ['zoom'],
                6,
                3,
                12,
                6,
                16,
                9
              ],
              'circle-color': [
                'match',
                ['get', 'status'],
                'reviewed',
                COLOURS.reviewed,
                'review',
                COLOURS.review,
                COLOURS.ok
              ],
              'circle-stroke-color': '#ffffff',
              'circle-stroke-width': 1.5,
              'circle-opacity': 0.9
            }
          });
          map.on('mouseenter', LAYER, (event) => {
            if (!map) return;
            map.getCanvas().style.cursor = 'pointer';
            const feature = event.features?.[0];
            if (!feature) return;
            const props = feature.properties as {
              name: string;
              address: string;
            };
            const text = document.createElement('div');
            text.className = 'map-popup';
            const strong = document.createElement('strong');
            strong.textContent = props.name;
            const span = document.createElement('span');
            span.textContent = props.address;
            text.append(strong, span);
            popup
              ?.setLngLat(
                (feature.geometry as GeoJSON.Point).coordinates as [
                  number,
                  number
                ]
              )
              .setDOMContent(text)
              .addTo(map);
          });
          map.on('mouseleave', LAYER, () => {
            if (!map) return;
            map.getCanvas().style.cursor = '';
            popup?.remove();
          });
          map.on('click', LAYER, (event) => {
            const id = event.features?.[0]?.properties?.id as
              string | undefined;
            const row = rows.find((r) => r.id === id);
            if (row) onopen(row);
          });
          ready = true;
          fit(plotted);
        });
      } catch (e) {
        failed = (e as Error).message || 'The map could not be loaded.';
      }
    })();
    return () => {
      disposed = true;
      popup?.remove();
      map?.remove();
      map = undefined;
    };
  });

  $effect(() => {
    const data = plotted;
    if (!ready || !map) return;
    (map.getSource(SOURCE) as GeoJSONSource | undefined)?.setData(data);
    fit(data);
  });
</script>

<div class="map-status" role="status">
  <span
    >{t('{count} of {total} records have coordinates', {
      count: number(plotted.features.length),
      total: number(rows.length)
    })}
    {#if missing > 0}<span class="dot-divider">·</span>
      {t(
        missing === 1
          ? '1 without coordinates is only shown in the list'
          : '{count} without coordinates are only shown in the list',
        { count: number(missing) }
      )}
    {/if}
  </span>
  <span class="map-legend" aria-label={t('Marker colours')}>
    <i style:background={COLOURS.review}></i>{t('Needs review')}
    <i style:background={COLOURS.reviewed}></i>{t('Reviewed')}
    <i style:background={COLOURS.ok}></i>{t('Checks passed')}
  </span>
</div>
<div class="map-frame">
  <div
    class="map-canvas"
    bind:this={container}
    aria-label={t('Map of business locations')}
  ></div>
  {#if failed}<div class="map-message" role="alert">
      <strong>{t('Map unavailable')}</strong><span
        >{translateMessage(locale, failed)}</span
      >
    </div>{:else if !ready}<div class="map-message">
      <span class="loading loading-spinner loading-sm"></span>{t(
        'Loading map\u2026'
      )}
    </div>{/if}
  {#if ready && !plotted.features.length}<div class="map-message">
      <strong>{t('No records with coordinates in this view')}</strong><span
        >{t('Try another filter or search.')}</span
      >
    </div>{/if}
</div>
