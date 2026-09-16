<script lang="ts">
  export let name = 'file';
  export let size = 20;
  const paths: Record<string, string> = {
    upload: 'M12 16V4m-5 5 5-5 5 5M4 16v4a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-4',
    file: 'M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8zM14 2v6h6M8 13h8M8 17h5',
    check: 'm5 12 4 4L19 6',
    arrow: 'M4 12h16m-6-6 6 6-6 6',
    download: 'M12 3v12m-5-5 5 5 5-5M4 17v4h16v-4',
    shield: 'M12 3 3 7v5c0 5 9 9 9 9s9-4 9-9V7zM8 12l3 3 5-6',
    chevron: 'm9 5 7 7-7 7',
    close: 'm6 6 12 12M6 18 18 6',
    search: 'M21 21l-5-5M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0',
    layers: 'm12 3 10 6-10 6L2 9zm-10 12 10 6 10-6M2 12l10 6 10-6',
    mail: 'M3 5h18v14H3zm0 1 9 7 9-7',
    pin: 'M20 10c0 6-8 12-8 12S4 16 4 10a8 8 0 1 1 16 0ZM15 10a3 3 0 1 1-6 0 3 3 0 0 1 6 0',
    info: 'M12 8h.01M12 11v6M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',
    back: 'M20 12H4m6-6-6 6 6 6',
    edit: 'm16 3 5 5-12 12-6 1 1-6zM13 6l5 5',
    clock: 'M12 6v6l4 2M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',
    external: 'M14 3h7v7m0-7L10 14M10 3H3v18h18v-7'
  };
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="1.65"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"><path d={paths[name] || paths.file} /></svg
>
