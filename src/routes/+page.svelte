<script lang="ts" module>
  function focusEditor(node: HTMLElement) {
    const previous = document.activeElement as HTMLElement | null;
    node.focus();
    const oldOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const handle = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        const button = node.querySelector<HTMLButtonElement>(
          '[aria-label="Close record"]'
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
  ><title>KBO Review — Business data, in order</title><meta
    name="description"
    content="Organise KBO business records, review data quality and export a clear, editable dataset."
  /><meta name="referrer" content="no-referrer" /></svelte:head
>

<div class="app-shell">
  <header class="topbar">
    <a class="brand" href="/" aria-label="KBO Review home"
      ><span class="brand-mark">k<span>↗</span></span><span
        >KBO <strong>Review</strong></span
      ></a
    >
    <div class="header-right">
      <span class="workspace-label"
        ><span class="status-dot"></span> Business data workspace</span
      ><button class="help-button" onclick={() => (showGuide = !showGuide)}
        ><Icon name="info" size={17} /> How it works</button
      ><span class="language-label">EN</span>
    </div>
  </header>
  <div class="context-bar">
    <span>Crossroads Bank for Enterprises</span><span class="context-separator"
      >/</span
    ><span>Data review</span><span class="prototype-label"
      >Independent workspace</span
    >
  </div>
  <main class:wide={screen === 'results'}>
    <nav class="steps-nav" aria-label="File review progress">
      {#each ['Upload your file', 'Check & organise', 'Review & export'] as label, i}
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
          <strong>From one export to a usable dataset</strong>
          <p>
            Upload an authorised KBO export. We organise its columns and flag
            missing data, duplicate identifiers and status checks. Review any
            corrections, then download your full dataset. Original source fields
            stay with every record.
          </p>
          <p>
            Google Maps lookups are optional suggestions. They do not verify
            legal registration or provide email addresses.
          </p>
        </div>
        <button
          class="icon-button"
          aria-label="Close help"
          onclick={() => (showGuide = false)}><Icon name="close" /></button
        >
      </aside>
    {/if}
    {#if error}<div class="error-message" role="alert">
        <Icon name="info" /><span>{error}</span
        >{#if screen === 'processing'}<button
            class="btn btn-sm"
            onclick={() => {
              error = '';
              const id = new URL(location.href).searchParams.get('job');
              if (id) poll(id);
            }}>Retry</button
          ><button class="btn btn-sm" onclick={reset}>Start again</button>{/if}
      </div>{/if}

    {#if screen === 'upload'}
      <section class="intro">
        <p class="eyebrow">A clearer view of your business data</p>
        <h1 bind:this={heading} tabindex="-1">
          Good decisions start<br />with organised data.
        </h1>
        <p class="intro-copy">
          Turn your KBO export into a clear, reviewable dataset.<br
            class="desktop-break"
          /> Bring your file. We’ll help you put it in order.
        </p>
      </section>
      <div class="upload-layout">
        <section class="upload-panel" aria-label="Upload a business file">
          <div class="section-heading">
            <h2>Start with your file</h2>
            <span class="small-label">STEP 01</span>
          </div>
          <input
            bind:this={fileInput}
            type="file"
            disabled={!hydrated}
            accept=".csv,.json,.geojson"
            class="sr-only"
            tabindex="-1"
            aria-label="Upload file"
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
                >{(file.size / 1024).toFixed(1)} KB
                <span class="dot-divider">·</span> Ready to organise</span
              ><span class="browse-link">Choose a different file</span
              >{:else}<strong>Drop your business data here</strong><span
                >or <span class="browse-link">browse files</span> on your computer</span
              ><span class="file-types"
                >CSV <i></i> JSON <i></i> GeoJSON
                <span class="file-limit">Up to 10 MB</span></span
              >{/if}
          </button>
          <div class="example-row">
            <span>Want to see how it works?</span><a
              href="/example.csv"
              download><Icon name="download" size={15} /> Download an example</a
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
                ><strong>Compare with Google Maps</strong><span
                  >Look for address, phone and business-status suggestions.</span
                ></span
              ></label
            ><span
              class="badge connection-badge"
              class:connected={config.google}
              >{config.google ? 'Connected' : 'Not connected'}</span
            >
          </div>
          <details class="notification-details">
            <summary
              ><Icon name="mail" size={17} /> Get an email when it’s ready
              <span>Optional</span></summary
            ><label class="field-label" for="upload-email">Email address</label
            ><input
              id="upload-email"
              type="email"
              class="input"
              bind:value={email}
              disabled={!config.email}
              placeholder="you@municipality.be"
            />
            <p>
              {config.email
                ? 'We’ll send one completion notice for this task.'
                : 'Email notifications are available once a mail service is connected.'}
            </p>
          </details>
          <div class="upload-actions">
            <span
              ><Icon name="shield" size={16} /> Your original data stays intact</span
            ><button
              class="btn btn-primary"
              disabled={!file ||
                busy ||
                Boolean(email && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email))}
              onclick={start}
              >{#if busy}<span class="loading loading-spinner loading-xs"
                ></span>Reading your file{:else}Organise file<Icon
                  name="arrow"
                  size={17}
                />{/if}</button
            >
          </div>
        </section>
        <aside class="explanation">
          <span class="eyebrow">What happens next</span>
          <h2>Less sorting. <br />More clarity.</h2>
          <ol class="benefit-list">
            <li>
              <span class="benefit-number">01</span>
              <div>
                <h3>Bring everything together</h3>
                <p>
                  Business names, KBO numbers, addresses and coordinates, neatly
                  organised.
                </p>
              </div>
            </li>
            <li>
              <span class="benefit-number">02</span>
              <div>
                <h3>Know what needs attention</h3>
                <p>
                  Spot duplicate identifiers, missing information and records to
                  review.
                </p>
              </div>
            </li>
            <li>
              <span class="benefit-number">03</span>
              <div>
                <h3>Leave with a useful file</h3>
                <p>
                  Review, make corrections, and download as JSON, CSV or
                  GeoJSON.
                </p>
              </div>
            </li>
          </ol>
          <div class="source-note">
            <Icon name="info" size={18} />
            <p>
              Made for your KBO export.<br /><span
                >Use a file you are authorised to process. No copying from
                public search pages.</span
              >
            </p>
          </div>
        </aside>
      </div>
      <div class="bottom-note">
        <span class="mini-grid" aria-hidden="true">▦</span><span
          >All your columns. All your records. <strong
            >Nothing lost along the way.</strong
          ></span
        >
      </div>
    {:else if screen === 'processing'}
      <section class="intro processing-intro">
        <p class="eyebrow">Your file is in good hands</p>
        <h1 bind:this={heading} tabindex="-1">Putting the details in order.</h1>
        <p class="intro-copy">
          We’re checking the data in your file. Any uncertainty will be marked
          for review.
        </p>
      </section>
      <section class="processing-panel">
        <div class="processing-file">
          <span class="file-icon"><Icon name="file" size={25} /></span>
          <div>
            <strong
              >{job?.filename ?? file?.name ?? 'Opening your task…'}</strong
            >
            <p>
              {job ? job.total.toLocaleString() + ' records' : 'Reading file'}
            </p>
          </div>
          <span class="loading loading-spinner"></span>
        </div>
        <div class="progress-label">
          <span
            >{job?.phase === 'google'
              ? 'Comparing Google Maps candidates'
              : 'Checking records'}</span
          ><strong>{progress}%</strong>
        </div>
        <progress
          class="progress progress-primary"
          value={progress}
          max="100"
          aria-label="Records processed"
        ></progress>
        <p class="progress-count">
          {job?.progress.toLocaleString() ?? 0} of {job?.total.toLocaleString() ??
            '—'} records processed
        </p>
        <div class="validation-layers">
          <div>
            <span class="layer-status done"
              ><Icon name="check" size={16} /></span
            >
            <div>
              <strong>Read & organise</strong>
              <p>Identify columns and preserve source data</p>
            </div>
            <span>Complete</span>
          </div>
          <div>
            <span class="layer-status"><Icon name="layers" size={16} /></span>
            <div>
              <strong>Check data quality</strong>
              <p>Identifiers, duplicate records and missing fields</p>
            </div>
            <span>In progress</span>
          </div>
          <div>
            <span class="layer-status muted"><Icon name="pin" size={16} /></span
            >
            <div>
              <strong>Compare business information</strong>
              <p>
                {job?.enrich
                  ? 'Google Maps candidates are suggestions to review'
                  : 'Google Maps comparison was not requested'}
              </p>
            </div>
            <span>{job?.enrich ? 'Queued' : 'Skipped'}</span>
          </div>
        </div>
        <div class="email-callout">
          <Icon name="mail" size={24} />
          <div>
            <h3>No need to keep watching.</h3>
            <p>
              {job?.email
                ? 'A completion notice is requested for ' + job.email
                : config.email
                  ? 'Leave your email and we’ll let you know when it’s ready.'
                  : 'This task keeps running if you close the tab. Bookmark this page to return.'}
            </p>
            {#if config.email && !job?.email}<form onsubmit={saveEmail}>
                <input
                  class="input"
                  type="email"
                  required
                  placeholder="you@municipality.be"
                  aria-label="Notification email"
                  bind:value={email}
                /><button class="btn btn-primary" disabled={saving}
                  >Notify me</button
                >
              </form>{/if}{#if notificationMessage}<p role="status">
                {notificationMessage}
              </p>{/if}
          </div>
        </div>
      </section>
    {:else if job}
      <section class="results-heading">
        <div>
          <p class="eyebrow"><Icon name="check" size={15} /> File organised</p>
          <h1 bind:this={heading} tabindex="-1">Ready for a closer look.</h1>
          <p class="intro-copy">
            {job.filename} <span class="dot-divider">·</span>
            {job.total.toLocaleString()} records, original data preserved
          </p>
        </div>
        <button class="btn btn-outline new-file" onclick={reset}
          ><Icon name="upload" size={16} /> Upload another file</button
        >
      </section>
      <div class="result-summary">
        <div class="summary-text">
          <span class="success-icon"><Icon name="check" size={19} /></span>
          <p>
            <strong>Your file is organised.</strong>
            {attention > 0
              ? `${attention} records need a closer look.`
              : 'No unresolved checks remain.'}<span
              >{job.enrich
                ? 'Google Maps candidates require a human check.'
                : 'Source data checked. Google Maps comparison was not run.'}</span
            >
          </p>
        </div>
        <div class="export-controls">
          <button class="btn btn-primary" onclick={() => exportFile('json')}
            ><Icon name="download" size={17} /> Download JSON</button
          >
          <details class="dropdown dropdown-end">
            <summary class="btn btn-outline" aria-label="Other download formats"
              ><span>Other formats</span><span class="down-chevron">⌄</span
              ></summary
            >
            <ul class="menu dropdown-content">
              <li>
                <button onclick={() => exportFile('csv')}>Download CSV</button>
              </li>
              <li>
                <button onclick={() => exportFile('geojson')}
                  >Download GeoJSON</button
                >
              </li>
            </ul>
          </details>
        </div>
      </div>
      <section class="records-panel" aria-label="Business records">
        <div class="records-toolbar">
          <div class="filter-tabs" role="group" aria-label="Filter records">
            <button
              class:active={filter === 'all'}
              onclick={() => (filter = 'all')}
              >All records <span>{rows.length}</span></button
            ><button
              class:active={filter === 'review'}
              onclick={() => (filter = 'review')}
              >Needs review <span class="amber-count">{attention}</span></button
            ><button
              class:active={filter === 'reviewed'}
              onclick={() => (filter = 'reviewed')}
              >Reviewed <span>{reviewed}</span></button
            ><button
              class:active={filter === 'missing'}
              onclick={() => (filter = 'missing')}>Missing contact info</button
            >
          </div>
          <div class="toolbar-right">
            <label class="search-input"
              ><Icon name="search" size={17} /><input
                aria-label="Search businesses"
                bind:value={query}
                placeholder="Find a name, number or address…"
              /></label
            >
            <div class="view-toggle" role="group" aria-label="View as">
              <button
                class:active={view === 'list'}
                aria-pressed={view === 'list'}
                onclick={() => (view = 'list')}
                ><Icon name="layers" size={14} /> List</button
              ><button
                class:active={view === 'map'}
                aria-pressed={view === 'map'}
                disabled={!config.mapbox}
                title={config.mapbox
                  ? 'Show records on a map'
                  : 'Set MAPBOX_ACCESS_TOKEN to enable the map view'}
                onclick={() => (view = 'map')}
                ><Icon name="pin" size={14} /> Map</button
              >
            </div>
          </div>
        </div>
        {#if view === 'map' && config.mapbox}
          <MapView rows={filtered} token={config.mapbox} onopen={openEditor} />
        {:else}
          <div class="table-scroll">
            <table class="table">
              <thead
                ><tr
                  ><th>Business / identifier</th><th>Address</th><th
                    >Source status</th
                  ><th>Review</th><th><span class="sr-only">Actions</span></th
                  ></tr
                ></thead
              ><tbody
                >{#each visible as row}<tr
                    ><td
                      ><strong>{row.name || 'Unnamed business'}</strong><span
                        class="row-secondary"
                        >{row.number || 'No identifier'}
                        <span class="dot-divider">·</span>
                        {row.kind === 'establishment'
                          ? 'Establishment'
                          : 'Enterprise'}</span
                      ></td
                    ><td class="address-cell"
                      >{row.address || 'No address provided'}</td
                    ><td
                      ><span class="source-status"
                        >{row.status || 'Not provided'}</span
                      ></td
                    ><td
                      >{#if row.reviewed}<span class="badge reviewed-badge"
                          ><Icon name="check" size={12} />Reviewed</span
                        >{:else if needsReview(row)}<span
                          class="badge attention-badge">Needs review</span
                        >{:else}<span class="badge neutral-badge"
                          >Checks passed</span
                        >{/if}<span class="row-secondary"
                        >{row.issues[0] ||
                          (row.google
                            ? 'Google Maps candidate'
                            : row.googleError
                              ? 'Google Maps unavailable'
                              : 'Source checks only')}</span
                      ></td
                    ><td
                      ><button
                        class="review-button"
                        onclick={() => openEditor(row)}
                        aria-label={'Review ' + row.name}
                        >Review <Icon name="chevron" size={15} /></button
                      ></td
                    ></tr
                  >{/each}</tbody
              >
            </table>
            {#if filtered.length === 0}<div class="empty-state">
                <Icon name="search" size={26} />
                <h3>No records match this view</h3>
                <p>Try another search or return to all records.</p>
                <button
                  class="btn btn-outline"
                  onclick={() => {
                    query = '';
                    filter = 'all';
                  }}>Show all records</button
                >
              </div>{/if}
          </div>
          <div class="pagination">
            <span
              >{filtered.length
                ? `${page * 20 + 1}–${Math.min((page + 1) * 20, filtered.length)} of ${filtered.length} records`
                : '0 records'}</span
            >
            <div>
              <button
                class="btn btn-sm btn-ghost"
                disabled={page === 0}
                onclick={() => page--}
                ><Icon name="back" size={15} />Previous</button
              ><button
                class="btn btn-sm btn-ghost"
                disabled={(page + 1) * 20 >= filtered.length}
                onclick={() => page++}
                >Next<Icon name="arrow" size={15} /></button
              >
            </div>
          </div>
        {/if}
      </section>
      <div class="results-footnote">
        <p>
          <Icon name="shield" size={16} /> Corrections are saved separately. Every
          download includes original source fields.
        </p>
        <span
          >{job.email
            ? 'Email notification: ' + job.notification
            : 'No completion email requested'}</span
        >
      </div>
      {#if config.email && !job.email}<form
          class="completed-notification"
          onsubmit={saveEmail}
        >
          <Icon name="mail" size={20} /><label for="result-email"
            >Email me a completion notice</label
          ><input
            id="result-email"
            class="input"
            type="email"
            required
            placeholder="you@municipality.be"
            bind:value={email}
          /><button class="btn btn-outline" disabled={saving}
            >Send notice</button
          >
        </form>{/if}
    {/if}
  </main>
  <footer>
    <span class="footer-brand"
      >KBO Review <span>Business data, in order.</span></span
    >
    <div>
      <span class="belgian-mark" aria-hidden="true"><i></i><i></i><i></i></span> Built
      for local public services
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
          Record {editor.id} <span class="dot-divider">/</span>
          {editor.kind}
        </p>
        <h2 id="editor-title">Review business details</h2>
      </div>
      <button
        class="icon-button"
        disabled={saving}
        onclick={() => (editor = null)}
        aria-label="Close record"><Icon name="close" /></button
      >
    </div>
    <form onsubmit={saveEdit} class="editor-form">
      <p class="editor-number">
        {editor.number}
        {#if editor.enterprise && editor.enterprise !== editor.number}<span
            >Parent enterprise: {editor.enterprise}</span
          >{/if}
      </p>
      {#if editor.issues.length}<div class="review-issues">
          <strong>Check before using this record</strong>
          <ul>
            {#each editor.issues as issue}<li>{issue}</li>{/each}
          </ul>
        </div>{/if}
      <label class="field-label"
        >Business name<input class="input" bind:value={editor.name} /></label
      ><label class="field-label"
        >Address<input class="input" bind:value={editor.address} /></label
      >
      <div class="two-fields">
        <label class="field-label"
          >Phone<input
            class="input"
            type="tel"
            bind:value={editor.phone}
            placeholder="Not provided"
          /></label
        ><label class="field-label"
          >Email<input
            class="input"
            type="email"
            bind:value={editor.email}
            placeholder="Not provided"
          /></label
        >
      </div>
      <label class="field-label"
        >Website<input
          class="input"
          bind:value={editor.website}
          placeholder="Not provided"
        /></label
      >
      <div class="registered-status">
        <span>Registered status in source</span><strong
          >{editor.status || 'Not provided'}</strong
        >
        <p>This is the uploaded value, not a live KBO verification.</p>
      </div>
      {#if editor.google}<div class="google-candidate">
          <p class="eyebrow">Google Maps · Unconfirmed candidate</p>
          <h3>{editor.google.displayName.text}</h3>
          <p>{editor.google.formattedAddress}</p>
          <dl>
            <dt>Business status</dt>
            <dd>{editor.google.businessStatus || 'Not provided'}</dd>
            <dt>Phone</dt>
            <dd>{editor.google.internationalPhoneNumber || 'Not provided'}</dd>
            <dt>Website</dt>
            <dd>{editor.google.websiteUri || 'Not provided'}</dd>
          </dl>
          <p>
            Confirm this is the same establishment before making any
            corrections. Google closure status is not legal registration status.
          </p>
          {#if editor.google.googleMapsUri.startsWith('https://')}<a
              href={editor.google.googleMapsUri}
              target="_blank"
              rel="noreferrer"
              >Open in Google Maps <Icon name="external" size={14} /></a
            >{/if}
        </div>{:else if editor.googleError}<div class="review-issues">
          {editor.googleError}. No external information was confirmed.
        </div>{/if}
      <label class="field-label"
        >Review notes<textarea
          class="textarea"
          rows="3"
          bind:value={editor.notes}
          placeholder="Record your source or explain a correction…"
        ></textarea></label
      ><label class="review-checkbox"
        ><input
          class="checkbox checkbox-sm"
          type="checkbox"
          bind:checked={editor.reviewed}
        /><span>I have reviewed this record</span></label
      >
      <details class="source-details">
        <summary>View original source fields</summary>
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
          {editorError}
        </p>{/if}
      <div class="editor-actions">
        <button
          class="btn btn-outline"
          type="button"
          disabled={saving}
          onclick={() => (editor = null)}>Cancel</button
        ><button class="btn btn-primary" disabled={saving}
          >{saving ? 'Saving…' : 'Save changes'}<Icon
            name="check"
            size={17}
          /></button
        >
      </div>
    </form>
  </div>
{/if}
