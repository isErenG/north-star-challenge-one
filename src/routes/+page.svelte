<script lang="ts" module>
  function focusEditor(node: HTMLElement) {
    const previous = document.activeElement as HTMLElement | null;
    node.focus();
    const oldOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const handle = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        const button = node.querySelector<HTMLButtonElement>(
          '[data-close-record]'
        );
        button?.click();
      }
      if (event.key !== 'Tab') return;
      const elements = Array.from(
        node.querySelectorAll<HTMLElement>(
          'button:not(:disabled), input, textarea, summary, a[href]'
        )
      ).filter((el) => el.getClientRects().length > 0);
      const first = elements[0],
        last = elements.at(-1);
      if (
        event.shiftKey &&
        (document.activeElement === first || document.activeElement === node)
      ) {
        event.preventDefault();
        last?.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first?.focus();
      }
    };
    node.addEventListener('keydown', handle);
    return {
      destroy() {
        document.body.style.overflow = oldOverflow;
        node.removeEventListener('keydown', handle);
        previous?.focus();
      }
    };
  }
</script>

<script lang="ts">
  import { onMount } from 'svelte';
  import {
    localeCookie,
    translate,
    translateMessage,
    resolveLocale,
    type Locale
  } from '$lib/i18n';
  let locale = $state<Locale>('nl');
  const numberFormat = $derived(
    new Intl.NumberFormat(locale === 'nl' ? 'nl-BE' : 'en-GB')
  );
  const t = (message: string, values?: Record<string, string | number>) =>
    translate(locale, message, values);
  const message = (value: string) => translateMessage(locale, value);
  const number = (value: number) => numberFormat.format(value);
  function changeLanguage(value: string) {
    locale = resolveLocale(value);
    document.cookie = `${localeCookie}=${locale}; Path=/; Max-Age=31536000; SameSite=Lax${location.protocol === 'https:' ? '; Secure' : ''}`;
  }
  $effect(() => {
    document.documentElement.lang = locale;
  });
  import Icon from '$lib/Icon.svelte';
  import MapView from '$lib/MapView.svelte';
  import { download, exportJob, needsReview } from '$lib/export';
  import type { Job, Business } from '$lib/types';
  let screen = $state<'upload' | 'processing' | 'results'>('upload');
  let config = $state({ google: false, email: false, mapbox: '' });
  let file = $state<File | null>(null);
  let dragover = $state(false);
  let hydrated = $state(false);
  let busy = $state(false);
  let error = $state('');
  let email = $state('');
  let enrich = $state(false);
  let job = $state<Job | null>(null);
  let query = $state('');
  let filter = $state('all');
  let view = $state<'list' | 'map'>('list');
  let page = $state(0);
  let editor = $state<Business | null>(null);
  let editorError = $state('');
  let saving = $state(false);
  let showGuide = $state(false);
  let notificationMessage = $state('');
  let fileInput = $state<HTMLInputElement>();
  let timer: ReturnType<typeof setTimeout>;
  let disposed = false;
  let heading = $state<HTMLHeadingElement>();
  let rows = $derived(job?.records ?? []);
  let attention = $derived(rows.filter(needsReview).length);
  let reviewed = $derived(rows.filter((row) => row.reviewed).length);
  let filtered = $derived(
    rows.filter((row) => {
      const matches = [row.name, row.number, row.enterprise, row.address]
        .join(' ')
        .toLowerCase()
        .includes(query.toLowerCase());
      return (
        matches &&
        (filter === 'all' ||
          (filter === 'review' && needsReview(row)) ||
          (filter === 'reviewed' && row.reviewed) ||
          (filter === 'missing' && (!row.phone || !row.email)))
      );
    })
  );
  let visible = $derived(filtered.slice(page * 20, (page + 1) * 20));
  let progress = $derived(
    job ? Math.round((job.progress / job.total) * 100) : 0
  );
  $effect(() => {
    query;
    filter;
    page = 0;
  });
  async function api(path: string, options?: RequestInit) {
    const response = await fetch('/api/' + path, options);
    const data = await response.json();
    if (!response.ok)
      throw new Error(data.error || 'Something went wrong. Please try again.');
    return data;
  }
  onMount(() => {
    locale = resolveLocale(
      document.cookie
        .split('; ')
        .find((value) => value.startsWith(localeCookie + '='))
        ?.split('=')[1]
    );
    hydrated = true;
    api('config')
      .then((data) => (config = data))
      .catch(
        () =>
          (error =
            'The processing service is unavailable. Please start the app with npm run dev.')
      );
    const id = new URL(location.href).searchParams.get('job');
    if (id && /^[a-f0-9]{48}$/.test(id)) {
      screen = 'processing';
      poll(id);
    }
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  });
  function choose(selected: File | null) {
    error = '';
    if (!selected) return;
    if (!/\.(csv|json|geojson)$/i.test(selected.name)) {
      error = 'Choose a CSV, JSON or GeoJSON file.';
      return;
    }
    if (selected.size > 10 * 1024 * 1024) {
      error = 'This file is too large. Choose a file smaller than 10 MB.';
      return;
    }
    if (!selected.size) {
      error = 'This file is empty. Choose a file that contains records.';
      return;
    }
    file = selected;
  }
  async function start() {
    if (!file || busy) return;
    busy = true;
    error = '';
    try {
      const form = new FormData();
      form.append('file', file);
      form.append('email', email);
      form.append('enrich', String(enrich));
      const data = await api('jobs', { method: 'POST', body: form });
      history.replaceState(null, '', '?job=' + data.id);
      screen = 'processing';
      heading?.focus();
      await poll(data.id);
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }
  async function poll(id: string) {
    try {
      const data: Job = await api('jobs/' + id);
      if (disposed) return;
      job = data;
      if (data.state === 'done') {
        screen = 'results';
        error = '';
        if (['pending', 'sending'].includes(data.notification))
          timer = setTimeout(() => poll(id), 1500);
        return;
      }
      if (data.state === 'failed') {
        error =
          data.error || 'Processing failed. Please upload the file again.';
        return;
      }
      timer = setTimeout(() => poll(id), 750);
    } catch (e) {
      if (!disposed) error = (e as Error).message;
    }
  }
  function reset() {
    clearTimeout(timer);
    screen = 'upload';
    file = null;
    job = null;
    error = '';
    query = '';
    filter = 'all';
    view = 'list';
    page = 0;
    notificationMessage = '';
    editor = null;
    history.replaceState(null, '', '/');
  }
  function exportFile(format: 'json' | 'csv' | 'geojson') {
    if (!job) return;
    download(
      exportJob(job, format),
      job.filename.replace(/\.[^.]+$/, '') + '-reviewed.' + format,
      format === 'csv' ? 'text/csv;charset=utf-8' : 'application/json'
    );
  }
  async function saveEdit(event: SubmitEvent) {
    event.preventDefault();
    if (!job || !editor) return;
    saving = true;
    editorError = '';
    try {
      job = await api('jobs/' + job.id, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          record: {
            id: editor.id,
            name: editor.name,
            address: editor.address,
            phone: editor.phone,
            email: editor.email,
            website: editor.website,
            notes: editor.notes,
            reviewed: editor.reviewed
          }
        })
      });
      editor = null;
    } catch (e) {
      editorError = (e as Error).message;
    } finally {
      saving = false;
    }
  }
  async function saveEmail(event?: SubmitEvent) {
    event?.preventDefault();
    if (!job) return;
    saving = true;
    error = '';
    try {
      job = await api('jobs/' + job.id, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email })
      });
      notificationMessage = 'Saved. We’ll email you when your file is ready.';
      if (job?.state === 'done') poll(job.id);
    } catch (e) {
      error = (e as Error).message;
    } finally {
      saving = false;
    }
  }
  function openEditor(row: Business) {
    editor = structuredClone($state.snapshot(row));
    editorError = '';
  }
</script>

<svelte:head
  ><title>{t('KBO Review — Business data, in order')}</title><meta
    name="description"
    content={t(
      'Organise KBO business records, review data quality and export a clear, editable dataset.'
    )}
  /><meta name="referrer" content="no-referrer" /></svelte:head
>

<div class="app-shell">
  <header class="topbar">
    <a class="brand" href="/" aria-label={t('KBO Review home')}
      ><span class="brand-mark">k<span>↗</span></span><span
        >KBO <strong>Review</strong></span
      ></a
    >
    <div class="header-right">
      <span class="workspace-label"
        ><span class="status-dot"></span> {t('Business data workspace')}</span
      ><button class="help-button" onclick={() => (showGuide = !showGuide)}
        ><Icon name="info" size={17} /> {t('How it works')}</button
      ><select
        class="language-switcher"
        aria-label={t('Language')}
        value={locale}
        onchange={(event) => changeLanguage(event.currentTarget.value)}
      >
        <option value="nl">NL</option><option value="en">EN</option>
      </select>
    </div>
  </header>
  <div class="context-bar">
    <span>{t('Crossroads Bank for Enterprises')}</span><span
      class="context-separator">/</span
    ><span>{t('Data review')}</span><span class="prototype-label"
      >{t('Independent workspace')}</span
    >
  </div>
  <main class:wide={screen === 'results'}>
    <nav class="steps-nav" aria-label={t('File review progress')}>
      {#each [t('Upload your file'), t('Check & organise'), t('Review & export')] as label, i}
        {@const active =
          screen === 'upload' ? 0 : screen === 'processing' ? 1 : 2}
        <div
          class:current={active === i}
          class:complete={active > i}
          class="step-item"
          aria-current={active === i ? 'step' : undefined}
        >
          <span class="step-number"
            >{#if active > i}<Icon name="check" size={15} />{:else}{i +
                1}{/if}</span
          ><span>{label}</span>
        </div>
        {#if i < 2}<span class="step-line"></span>{/if}
      {/each}
    </nav>
    {#if showGuide}
      <aside class="guide">
        <div>
          <strong>{t('From one export to a usable dataset')}</strong>
          <p>
            {t(
              'Upload an authorised KBO export. We organise its columns and flag missing data, duplicate identifiers and status checks. Review any corrections, then download your full dataset. Original source fields stay with every record.'
            )}
          </p>
          <p>
            {t(
              'Google Maps lookups are optional suggestions. They do not verify legal registration or provide email addresses.'
            )}
          </p>
        </div>
        <button
          class="icon-button"
          aria-label={t('Close help')}
          onclick={() => (showGuide = false)}><Icon name="close" /></button
        >
      </aside>
    {/if}
    {#if error}<div class="error-message" role="alert">
        <Icon name="info" /><span>{message(error)}</span
        >{#if screen === 'processing'}<button
            class="btn btn-sm"
            onclick={() => {
              error = '';
              const id = new URL(location.href).searchParams.get('job');
              if (id) poll(id);
            }}>{t('Retry')}</button
          ><button class="btn btn-sm" onclick={reset}>{t('Start again')}</button
          >{/if}
      </div>{/if}

    {#if screen === 'upload'}
      <section class="intro">
        <p class="eyebrow">{t('A clearer view of your business data')}</p>
        <h1 bind:this={heading} tabindex="-1">
          {t('Good decisions start')}<br />{t('with organised data.')}
        </h1>
        <p class="intro-copy">
          {t('Turn your KBO export into a clear, reviewable dataset.')}<br
            class="desktop-break"
          />
          {t('Bring your file. We’ll help you put it in order.')}
        </p>
      </section>
      <div class="upload-layout">
        <section class="upload-panel" aria-label={t('Upload a business file')}>
          <div class="section-heading">
            <h2>{t('Start with your file')}</h2>
            <span class="small-label">{t('STEP 01')}</span>
          </div>
          <input
            bind:this={fileInput}
            type="file"
            disabled={!hydrated}
            accept=".csv,.json,.geojson"
            class="sr-only"
            tabindex="-1"
            aria-label={t('Upload file')}
            onchange={(e) => choose(e.currentTarget.files?.[0] ?? null)}
          />
          <button
            class="dropzone"
            disabled={!hydrated}
            class:dragover
            class:hasfile={file}
            onclick={() => fileInput?.click()}
            ondragover={(e) => {
              e.preventDefault();
              dragover = true;
            }}
            ondragleave={() => (dragover = false)}
            ondrop={(e) => {
              e.preventDefault();
              dragover = false;
              if (e.dataTransfer?.files.length !== 1) {
                error = 'Please upload one file at a time.';
                return;
              }
              choose(e.dataTransfer.files[0]);
            }}
          >
            <span class="file-illustration" aria-hidden="true"
              ><span class="paper-back"></span><span class="paper-front"
                ><Icon name={file ? 'check' : 'file'} size={32} /><span
                  >{file
                    ? file.name.split('.').pop()?.toUpperCase()
                    : 'DATA'}</span
                ></span
              ><span class="upload-bubble"
                ><Icon name={file ? 'check' : 'upload'} size={16} /></span
              ></span
            >
            {#if file}<strong class="filename">{file.name}</strong><span
                >{(file.size / 1024).toLocaleString(
                  locale === 'nl' ? 'nl-BE' : 'en-GB',
                  { minimumFractionDigits: 1, maximumFractionDigits: 1 }
                )} KB
                <span class="dot-divider">·</span>
                {t('Ready to organise')}</span
              ><span class="browse-link">{t('Choose a different file')}</span
              >{:else}<strong>{t('Drop your business data here')}</strong><span
                >{t('or')} <span class="browse-link">{t('browse files')}</span>
                {t('on your computer')}</span
              ><span class="file-types"
                >CSV <i></i> JSON <i></i> GeoJSON
                <span class="file-limit">{t('Up to 10 MB')}</span></span
              >{/if}
          </button>
          <div class="example-row">
            <span>{t('Want to see how it works?')}</span><a
              href="/example.csv"
              download
              ><Icon name="download" size={15} /> {t('Download an example')}</a
            >
          </div>
          <div class="connection-option">
            <label
              ><input
                type="checkbox"
                class="checkbox checkbox-sm"
                bind:checked={enrich}
                disabled={!config.google}
              /><span
                ><strong>{t('Compare with Google Maps')}</strong><span
                  >{t(
                    'Look for address, phone and business-status suggestions.'
                  )}</span
                ></span
              ></label
            ><span
              class="badge connection-badge"
              class:connected={config.google}
              >{config.google ? t('Connected') : t('Not connected')}</span
            >
          </div>
          <details class="notification-details">
            <summary
              ><Icon name="mail" size={17} />
              {t('Get an email when it’s ready')}
              <span>{t('Optional')}</span></summary
            ><label class="field-label" for="upload-email"
              >{t('Email address')}</label
            ><input
              id="upload-email"
              type="email"
              class="input"
              bind:value={email}
              disabled={!config.email}
              placeholder={t('you@municipality.be')}
            />
            <p>
              {config.email
                ? t('We’ll send one completion notice for this task.')
                : t(
                    'Email notifications are available once a mail service is connected.'
                  )}
            </p>
          </details>
          <div class="upload-actions">
            <span
              ><Icon name="shield" size={16} />
              {t('Your original data stays intact')}</span
            ><button
              class="btn btn-primary"
              disabled={!file ||
                busy ||
                Boolean(email && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email))}
              onclick={start}
              >{#if busy}<span class="loading loading-spinner loading-xs"
                ></span>{t('Reading your file')}{:else}{t('Organise file')}<Icon
                  name="arrow"
                  size={17}
                />{/if}</button
            >
          </div>
        </section>
        <aside class="explanation">
          <span class="eyebrow">{t('What happens next')}</span>
          <h2>{t('Less sorting.')} <br />{t('More clarity.')}</h2>
          <ol class="benefit-list">
            <li>
              <span class="benefit-number">01</span>
              <div>
                <h3>{t('Bring everything together')}</h3>
                <p>
                  {t(
                    'Business names, KBO numbers, addresses and coordinates, neatly organised.'
                  )}
                </p>
              </div>
            </li>
            <li>
              <span class="benefit-number">02</span>
              <div>
                <h3>{t('Know what needs attention')}</h3>
                <p>
                  {t(
                    'Spot duplicate identifiers, missing information and records to review.'
                  )}
                </p>
              </div>
            </li>
            <li>
              <span class="benefit-number">03</span>
              <div>
                <h3>{t('Leave with a useful file')}</h3>
                <p>
                  {t(
                    'Review, make corrections, and download as JSON, CSV or GeoJSON.'
                  )}
                </p>
              </div>
            </li>
          </ol>
          <div class="source-note">
            <Icon name="info" size={18} />
            <p>
              {t('Made for your KBO export.')}<br /><span
                >{t(
                  'Use a file you are authorised to process. No copying from public search pages.'
                )}</span
              >
            </p>
          </div>
        </aside>
      </div>
      <div class="bottom-note">
        <span class="mini-grid" aria-hidden="true">▦</span><span
          >{t('All your columns. All your records.')}
          <strong>{t('Nothing lost along the way.')}</strong></span
        >
      </div>
    {:else if screen === 'processing'}
      <section class="intro processing-intro">
        <p class="eyebrow">{t('Your file is in good hands')}</p>
        <h1 bind:this={heading} tabindex="-1">
          {t('Putting the details in order.')}
        </h1>
        <p class="intro-copy">
          {t(
            'We’re checking the data in your file. Any uncertainty will be marked for review.'
          )}
        </p>
      </section>
      <section class="processing-panel">
        <div class="processing-file">
          <span class="file-icon"><Icon name="file" size={25} /></span>
          <div>
            <strong
              >{job?.filename ?? file?.name ?? t('Opening your task…')}</strong
            >
            <p>
              {job
                ? t('{count} records', { count: number(job.total) })
                : t('Reading file')}
            </p>
          </div>
          <span class="loading loading-spinner"></span>
        </div>
        <div class="progress-label">
          <span
            >{job?.phase === 'google'
              ? t('Comparing Google Maps candidates')
              : t('Checking records')}</span
          ><strong>{progress}%</strong>
        </div>
        <progress
          class="progress progress-primary"
          value={progress}
          max="100"
          aria-label={t('Records processed')}
        ></progress>
        <p class="progress-count">
          {t('{done} of {total} records processed', {
            done: number(job?.progress ?? 0),
            total: job ? number(job.total) : '—'
          })}
        </p>
        <div class="validation-layers">
          <div>
            <span class="layer-status done"
              ><Icon name="check" size={16} /></span
            >
            <div>
              <strong>{t('Read & organise')}</strong>
              <p>{t('Identify columns and preserve source data')}</p>
            </div>
            <span>{t('Complete')}</span>
          </div>
          <div>
            <span class="layer-status"><Icon name="layers" size={16} /></span>
            <div>
              <strong>{t('Check data quality')}</strong>
              <p>{t('Identifiers, duplicate records and missing fields')}</p>
            </div>
            <span>{t('In progress')}</span>
          </div>
          <div>
            <span class="layer-status muted"><Icon name="pin" size={16} /></span
            >
            <div>
              <strong>{t('Compare business information')}</strong>
              <p>
                {job?.enrich
                  ? t('Google Maps candidates are suggestions to review')
                  : t('Google Maps comparison was not requested')}
              </p>
            </div>
            <span>{job?.enrich ? t('Queued') : t('Skipped')}</span>
          </div>
        </div>
        <div class="email-callout">
          <Icon name="mail" size={24} />
          <div>
            <h3>{t('No need to keep watching.')}</h3>
            <p>
              {job?.email
                ? t('A completion notice is requested for {email}', {
                    email: job.email
                  })
                : config.email
                  ? t(
                      'Leave your email and we’ll let you know when it’s ready.'
                    )
                  : t(
                      'This task keeps running if you close the tab. Bookmark this page to return.'
                    )}
            </p>
            {#if config.email && !job?.email}<form onsubmit={saveEmail}>
                <input
                  class="input"
                  type="email"
                  required
                  placeholder={t('you@municipality.be')}
                  aria-label={t('Notification email')}
                  bind:value={email}
                /><button class="btn btn-primary" disabled={saving}
                  >{t('Notify me')}</button
                >
              </form>{/if}{#if notificationMessage}<p role="status">
                {message(notificationMessage)}
              </p>{/if}
          </div>
        </div>
      </section>
    {:else if job}
      <section class="results-heading">
        <div>
          <p class="eyebrow">
            <Icon name="check" size={15} />
            {t('File organised')}
          </p>
          <h1 bind:this={heading} tabindex="-1">
            {t('Ready for a closer look.')}
          </h1>
          <p class="intro-copy">
            {job.filename} <span class="dot-divider">·</span>
            {number(job.total)}
            {t('records, original data preserved')}
          </p>
        </div>
        <button class="btn btn-outline new-file" onclick={reset}
          ><Icon name="upload" size={16} /> {t('Upload another file')}</button
        >
      </section>
      <div class="result-summary">
        <div class="summary-text">
          <span class="success-icon"><Icon name="check" size={19} /></span>
          <p>
            <strong>{t('Your file is organised.')}</strong>{' '}
            {attention > 0
              ? t(
                  attention === 1
                    ? '1 record needs a closer look.'
                    : '{count} records need a closer look.',
                  {
                    count: number(attention)
                  }
                )
              : t('No unresolved checks remain.')}<span
              >{job.enrich
                ? t('Google Maps candidates require a human check.')
                : t(
                    'Source data checked. Google Maps comparison was not run.'
                  )}</span
            >
          </p>
        </div>
        <div class="export-controls">
          <button class="btn btn-primary" onclick={() => exportFile('json')}
            ><Icon name="download" size={17} /> {t('Download JSON')}</button
          >
          <details class="dropdown dropdown-end">
            <summary
              class="btn btn-outline"
              aria-label={t('Other download formats')}
              ><span>{t('Other formats')}</span><span class="down-chevron"
                >⌄</span
              ></summary
            >
            <ul class="menu dropdown-content">
              <li>
                <button onclick={() => exportFile('csv')}
                  >{t('Download CSV')}</button
                >
              </li>
              <li>
                <button onclick={() => exportFile('geojson')}
                  >{t('Download GeoJSON')}</button
                >
              </li>
            </ul>
          </details>
        </div>
      </div>
      <section class="records-panel" aria-label={t('Business records')}>
        <div class="records-toolbar">
          <div
            class="filter-tabs"
            role="group"
            aria-label={t('Filter records')}
          >
            <button
              class:active={filter === 'all'}
              onclick={() => (filter = 'all')}
              >{t('All records')} <span>{number(rows.length)}</span></button
            ><button
              class:active={filter === 'review'}
              onclick={() => (filter = 'review')}
              >{t('Needs review')}
              <span class="amber-count">{number(attention)}</span></button
            ><button
              class:active={filter === 'reviewed'}
              onclick={() => (filter = 'reviewed')}
              >{t('Reviewed')} <span>{number(reviewed)}</span></button
            ><button
              class:active={filter === 'missing'}
              onclick={() => (filter = 'missing')}
              >{t('Missing contact info')}</button
            >
          </div>
          <div class="toolbar-right">
            <label class="search-input"
              ><Icon name="search" size={17} /><input
                aria-label={t('Search businesses')}
                bind:value={query}
                placeholder={t('Find a name, number or address…')}
              /></label
            >
            <div class="view-toggle" role="group" aria-label={t('View as')}>
              <button
                class:active={view === 'list'}
                aria-pressed={view === 'list'}
                onclick={() => (view = 'list')}
                ><Icon name="layers" size={14} /> {t('List')}</button
              ><button
                class:active={view === 'map'}
                aria-pressed={view === 'map'}
                disabled={!config.mapbox}
                title={config.mapbox
                  ? t('Show records on a map')
                  : t('Set MAPBOX_ACCESS_TOKEN to enable the map view')}
                onclick={() => (view = 'map')}
                ><Icon name="pin" size={14} /> {t('Map')}</button
              >
            </div>
          </div>
        </div>
        {#if view === 'map' && config.mapbox}
          {#key locale}<MapView
              {locale}
              rows={filtered}
              token={config.mapbox}
              onopen={openEditor}
            />{/key}
        {:else}
          <div class="table-scroll">
            <table class="table">
              <thead
                ><tr
                  ><th>{t('Business / identifier')}</th><th>{t('Address')}</th
                  ><th>{t('Source status')}</th><th>{t('Review')}</th><th
                    ><span class="sr-only">{t('Actions')}</span></th
                  ></tr
                ></thead
              ><tbody
                >{#each visible as row}<tr
                    ><td
                      ><strong>{row.name || t('Unnamed business')}</strong><span
                        class="row-secondary"
                        >{row.number || t('No identifier')}
                        <span class="dot-divider">·</span>
                        {row.kind === 'establishment'
                          ? t('Establishment')
                          : t('Enterprise')}</span
                      ></td
                    ><td class="address-cell"
                      >{row.address || t('No address provided')}</td
                    ><td
                      ><span class="source-status"
                        >{row.status || t('Not provided')}</span
                      ></td
                    ><td
                      >{#if row.reviewed}<span class="badge reviewed-badge"
                          ><Icon name="check" size={12} />{t('Reviewed')}</span
                        >{:else if needsReview(row)}<span
                          class="badge attention-badge"
                          >{t('Needs review')}</span
                        >{:else}<span class="badge neutral-badge"
                          >{t('Checks passed')}</span
                        >{/if}<span class="row-secondary"
                        >{message(row.issues[0] ?? '') ||
                          (row.google
                            ? t('Google Maps candidate')
                            : row.googleError
                              ? t('Google Maps unavailable')
                              : t('Source checks only'))}</span
                      ></td
                    ><td
                      ><button
                        class="review-button"
                        onclick={() => openEditor(row)}
                        aria-label={t('Review {name}', {
                          name: row.name || t('Unnamed business')
                        })}
                        >{t('Review')} <Icon name="chevron" size={15} /></button
                      ></td
                    ></tr
                  >{/each}</tbody
              >
            </table>
            {#if filtered.length === 0}<div class="empty-state">
                <Icon name="search" size={26} />
                <h3>{t('No records match this view')}</h3>
                <p>{t('Try another search or return to all records.')}</p>
                <button
                  class="btn btn-outline"
                  onclick={() => {
                    query = '';
                    filter = 'all';
                  }}>{t('Show all records')}</button
                >
              </div>{/if}
          </div>
          <div class="pagination">
            <span
              >{filtered.length
                ? t('{start}–{end} of {count} records', {
                    start: number(page * 20 + 1),
                    end: number(Math.min((page + 1) * 20, filtered.length)),
                    count: number(filtered.length)
                  })
                : t('0 records')}</span
            >
            <div>
              <button
                class="btn btn-sm btn-ghost"
                disabled={page === 0}
                onclick={() => page--}
                ><Icon name="back" size={15} />{t('Previous')}</button
              ><button
                class="btn btn-sm btn-ghost"
                disabled={(page + 1) * 20 >= filtered.length}
                onclick={() => page++}
                >{t('Next')}<Icon name="arrow" size={15} /></button
              >
            </div>
          </div>
        {/if}
      </section>
      <div class="results-footnote">
        <p>
          <Icon name="shield" size={16} />
          {t(
            'Corrections are saved separately. Every download includes original source fields.'
          )}
        </p>
        <span
          >{job.email
            ? t('Email notification: {status}', { status: t(job.notification) })
            : t('No completion email requested')}</span
        >
      </div>
      {#if config.email && !job.email}<form
          class="completed-notification"
          onsubmit={saveEmail}
        >
          <Icon name="mail" size={20} /><label for="result-email"
            >{t('Email me a completion notice')}</label
          ><input
            id="result-email"
            class="input"
            type="email"
            required
            placeholder={t('you@municipality.be')}
            bind:value={email}
          /><button class="btn btn-outline" disabled={saving}
            >{t('Send notice')}</button
          >
        </form>{/if}
    {/if}
  </main>
  <footer>
    <span class="footer-brand"
      >KBO Review <span>{t('Business data, in order.')}</span></span
    >
    <div>
      <span class="belgian-mark" aria-hidden="true"><i></i><i></i><i></i></span>
      {t('Built for local public services')}
    </div>
  </footer>
</div>

{#if editor}
  <div
    class="editor-backdrop"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget && !saving) editor = null;
    }}
  ></div>
  <div
    class="editor-panel"
    role="dialog"
    aria-modal="true"
    aria-labelledby="editor-title"
    tabindex="-1"
    use:focusEditor
  >
    <div class="editor-header">
      <div>
        <p class="eyebrow">
          {t('Record')}
          {editor.id} <span class="dot-divider">/</span>
          {t(editor.kind)}
        </p>
        <h2 id="editor-title">{t('Review business details')}</h2>
      </div>
      <button
        class="icon-button"
        disabled={saving}
        onclick={() => (editor = null)}
        data-close-record
        aria-label={t('Close record')}><Icon name="close" /></button
      >
    </div>
    <form onsubmit={saveEdit} class="editor-form">
      <p class="editor-number">
        {editor.number}
        {#if editor.enterprise && editor.enterprise !== editor.number}<span
            >{t('Parent enterprise:')} {editor.enterprise}</span
          >{/if}
      </p>
      {#if editor.issues.length}<div class="review-issues">
          <strong>{t('Check before using this record')}</strong>
          <ul>
            {#each editor.issues as issue}<li>{message(issue)}</li>{/each}
          </ul>
        </div>{/if}
      <label class="field-label"
        >{t('Business name')}<input
          class="input"
          bind:value={editor.name}
        /></label
      ><label class="field-label"
        >{t('Address')}<input
          class="input"
          bind:value={editor.address}
        /></label
      >
      <div class="two-fields">
        <label class="field-label"
          >{t('Phone')}<input
            class="input"
            type="tel"
            bind:value={editor.phone}
            placeholder={t('Not provided')}
          /></label
        ><label class="field-label"
          >{t('Email')}<input
            class="input"
            type="email"
            bind:value={editor.email}
            placeholder={t('Not provided')}
          /></label
        >
      </div>
      <label class="field-label"
        >{t('Website')}<input
          class="input"
          bind:value={editor.website}
          placeholder={t('Not provided')}
        /></label
      >
      <div class="registered-status">
        <span>{t('Registered status in source')}</span><strong
          >{editor.status || t('Not provided')}</strong
        >
        <p>{t('This is the uploaded value, not a live KBO verification.')}</p>
      </div>
      {#if editor.google}<div class="google-candidate">
          <p class="eyebrow">{t('Google Maps · Unconfirmed candidate')}</p>
          <h3>{editor.google.displayName.text}</h3>
          <p>{editor.google.formattedAddress}</p>
          <dl>
            <dt>{t('Business status')}</dt>
            <dd>{editor.google.businessStatus || t('Not provided')}</dd>
            <dt>{t('Phone')}</dt>
            <dd>
              {editor.google.internationalPhoneNumber || t('Not provided')}
            </dd>
            <dt>{t('Website')}</dt>
            <dd>{editor.google.websiteUri || t('Not provided')}</dd>
          </dl>
          <p>
            {t(
              'Confirm this is the same establishment before making any corrections. Google closure status is not legal registration status.'
            )}
          </p>
          {#if editor.google.googleMapsUri.startsWith('https://')}<a
              href={editor.google.googleMapsUri}
              target="_blank"
              rel="noreferrer"
              >{t('Open in Google Maps')} <Icon name="external" size={14} /></a
            >{/if}
        </div>{:else if editor.googleError}<div class="review-issues">
          {message(editor.googleError)}{t(
            '. No external information was confirmed.'
          )}
        </div>{/if}
      <label class="field-label"
        >{t('Review notes')}<textarea
          class="textarea"
          rows="3"
          bind:value={editor.notes}
          placeholder={t('Record your source or explain a correction…')}
        ></textarea></label
      ><label class="review-checkbox"
        ><input
          class="checkbox checkbox-sm"
          type="checkbox"
          bind:checked={editor.reviewed}
        /><span>{t('I have reviewed this record')}</span></label
      >
      <details class="source-details">
        <summary>{t('View original source fields')}</summary>
        <dl>
          {#each Object.entries(editor.source) as [key, value]}<dt>{key}</dt>
            <dd>
              {typeof value === 'object'
                ? JSON.stringify(value)
                : String(value ?? '') || '—'}
            </dd>{/each}
        </dl>
      </details>
      {#if editorError}<p class="error-message" role="alert">
          {message(editorError)}
        </p>{/if}
      <div class="editor-actions">
        <button
          class="btn btn-outline"
          type="button"
          disabled={saving}
          onclick={() => (editor = null)}>{t('Cancel')}</button
        ><button class="btn btn-primary" disabled={saving}
          >{saving ? t('Saving…') : t('Save changes')}<Icon
            name="check"
            size={17}
          /></button
        >
      </div>
    </form>
  </div>
{/if}
