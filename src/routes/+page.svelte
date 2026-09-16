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
  import {
    autoVerified,
    download,
    exportJob,
    needsReview,
    pendingSuggestions
  } from '$lib/export';
  import {
    indexBusiness,
    matchesSearch,
    industries,
    categories
  } from '$lib/search';
  import { placeIndustry } from '$lib/types';
  import type {
    Job,
    Business,
    Suggestion,
    SuggestionField,
    Stage,
    Step
  } from '$lib/types';
  // Labels stay English here and are translated with t() where they are shown.
  const SOURCES = [
    { key: 'google', label: 'Google Maps', preset: true },
    { key: 'website', label: 'Business website', preset: true },
    { key: 'openai', label: 'AI reconciliation', preset: true },
    { key: 'databe', label: 'data.be', preset: true },
    { key: 'goldenpages', label: 'Golden Pages', preset: false },
    { key: 'trendstop', label: 'Trendstop', preset: false },
    { key: 'vkbo', label: 'Registry re-check', preset: false },
    { key: 'companyweb_demo', label: 'Companyweb · Demo', preset: false }
  ] as const;
  const VERDICTS: Record<string, string> = {
    likely_active: 'Likely active',
    likely_ceased: 'Likely ceased',
    unclear: 'Unclear',
    skipped: 'Skipped',
    not_run: 'Queued',
    running: 'Verifying…'
  };
  const STAGES: { key: Stage; label: string }[] = [
    { key: 'checks', label: 'Data checks' },
    { key: 'google', label: 'Google Maps' },
    { key: 'website', label: 'Website' },
    { key: 'databe', label: 'data.be' },
    { key: 'goldenpages', label: 'Golden Pages' },
    { key: 'trendstop', label: 'Trendstop' },
    { key: 'vkbo', label: 'Registry' },
    { key: 'judge', label: 'AI verdict' },
    { key: 'extract', label: 'AI extraction' },
    { key: 'companyweb_demo', label: 'Companyweb · Demo' },
    { key: 'reconcile', label: 'Reconcile' }
  ];
  const STAGE_LABELS = Object.fromEntries(
    STAGES.map((stage) => [stage.key, stage.label])
  ) as Record<string, string>;
  const SOURCE_STAGES: Record<string, Stage> = {
    google: 'google',
    website: 'website',
    databe: 'databe',
    goldenpages: 'goldenpages',
    trendstop: 'trendstop',
    vkbo: 'vkbo',
    companyweb_demo: 'companyweb_demo'
  };
  const VERDICT_BADGE: Record<string, string> = {
    likely_active: 'reviewed-badge',
    likely_ceased: 'ceased-badge',
    unclear: 'attention-badge',
    skipped: 'neutral-badge',
    not_run: 'neutral-badge',
    running: 'running-badge'
  };
  const BOARD_LIMIT = 40;
  const FIELD_LABELS: Record<SuggestionField, string> = {
    name: 'Name',
    address: 'Address',
    phone: 'Phone',
    email: 'Email',
    website: 'Website',
    activity: 'Activity',
    status: 'Status'
  };
  let screen = $state<'upload' | 'processing' | 'results'>('upload');
  let config = $state<{
    google: boolean;
    email: boolean;
    mapbox: string;
    sources: Record<string, boolean>;
    defaultBudgetEur: number;
  }>({
    google: false,
    email: false,
    mapbox: '',
    sources: {},
    defaultBudgetEur: 5
  });
  let selectedSources = $state<Record<string, boolean>>({});
  let budget = $state(5);
  let verifying = $state(false);
  let accepting = $state('');
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
  let industry = $state('');
  let category = $state('');
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
  let expandedGroups = $state<Set<string>>(new Set());
  // Records folded into another one by internal/dedupe stay out of the main
  // list; they only show up nested under their canonical record.
  let rows = $derived((job?.records ?? []).filter((row) => !row.mergedInto));
  let mergedByCanonical = $derived(
    (job?.records ?? []).reduce((map, row) => {
      if (!row.mergedInto) return map;
      const group = map.get(row.mergedInto) ?? [];
      group.push(row);
      map.set(row.mergedInto, group);
      return map;
    }, new Map<string, Business[]>())
  );
  function toggleGroup(id: string) {
    const next = new Set(expandedGroups);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    expandedGroups = next;
  }
  let attention = $derived(rows.filter(needsReview).length);
  let reviewed = $derived(rows.filter((row) => row.reviewed).length);
  let ceased = $derived(rows.filter(isCeased).length);
  let anySourceReady = $derived(
    SOURCES.some((source) => config.sources[source.key])
  );
  let queued = $derived(rows.some(isQueued));
  let editorQueued = $derived(Boolean(editor && isQueued(editor)));
  let editorPending = $derived(
    editor ? (editor.suggestions ?? []).filter(acceptable) : []
  );
  let searchIndex = $derived(rows.map(indexBusiness));
  let industryOptions = $derived(
    [...new Set(searchIndex.flatMap((entry) => entry.industries))].sort(
      (a, b) => a.localeCompare(b, locale)
    )
  );
  let categoryOptions = $derived(
    [...new Set(searchIndex.flatMap((entry) => entry.categories))].sort(
      (a, b) => a.localeCompare(b, locale)
    )
  );
  let filtered = $derived(
    searchIndex
      .filter((entry) => {
        const row = entry.row;
        return (
          matchesSearch(entry.text, query) &&
          (!industry || entry.industries.includes(industry)) &&
          (!category || entry.categories.includes(category)) &&
          (filter === 'all' ||
            (filter === 'review' && needsReview(row)) ||
            (filter === 'reviewed' && row.reviewed) ||
            (filter === 'ceased' && isCeased(row)) ||
            (filter === 'missing' && (!row.phone || !row.email)))
        );
      })
      .map((entry) => entry.row)
  );
  let visible = $derived(filtered.slice(page * 20, (page + 1) * 20));
  let progress = $derived(
    job ? Math.round((job.progress / job.total) * 100) : 0
  );
  let tallies = $derived.by(() => {
    const counts = {
      likely_active: 0,
      likely_ceased: 0,
      unclear: 0,
      skipped: 0,
      running: 0,
      waiting: 0
    };
    for (const row of rows) {
      const verdict = row.verification?.verdict;
      if (!verdict || verdict === 'not_run') counts.waiting++;
      else if (verdict in counts) counts[verdict as keyof typeof counts]++;
    }
    return counts;
  });
  let jobStages = $derived.by(() => {
    const wanted = new Set<Stage>(['checks']);
    for (const source of job?.enrichment?.sources ?? []) {
      const stage = SOURCE_STAGES[source];
      if (stage) wanted.add(stage);
    }
    for (const row of rows)
      for (const step of row.verification?.trace ?? []) wanted.add(step.stage);
    wanted.add('judge');
    wanted.add('extract');
    wanted.add('reconcile');
    return STAGES.filter((stage) => wanted.has(stage.key));
  });
  let board = $derived(
    rows
      .map((row, index) => ({ row, index, rank: boardRank(row) }))
      .sort((a, b) => a.rank - b.rank || a.index - b.index)
      .slice(0, BOARD_LIMIT)
  );
  $effect(() => {
    query;
    industry;
    category;
    filter;
    page = 0;
  });
  function isCeased(row: Business) {
    return row.verification?.verdict === 'likely_ceased';
  }
  function isQueued(row: Business) {
    return (
      row.verification?.verdict === 'running' ||
      (row.verification?.verdict === 'not_run' &&
        row.verification.reason === 'queued')
    );
  }
  function boardRank(row: Business) {
    const verdict = row.verification?.verdict;
    if (verdict === 'running') return 0;
    if (!verdict || verdict === 'not_run') return 1;
    return 2;
  }
  function stageStep(row: Business, stage: Stage): Step | undefined {
    return row.verification?.trace?.findLast((step) => step.stage === stage);
  }
  function chipState(row: Business, stage: Stage) {
    const step = stageStep(row, stage);
    if (step) return step.status;
    return boardRank(row) === 2 ? 'idle' : 'waiting';
  }
  function chipTitle(row: Business, stage: Stage) {
    const step = stageStep(row, stage);
    const label = t(STAGE_LABELS[stage]);
    return step
      ? label + ': ' + (step.note ? message(step.note) : t(step.status))
      : label;
  }
  function duration(step: Step) {
    if (!step.endedAt) return '';
    const ms = Date.parse(step.endedAt) - Date.parse(step.startedAt);
    if (!Number.isFinite(ms) || ms < 0) return '';
    return ms < 1000 ? ms + ' ms' : (ms / 1000).toFixed(1) + ' s';
  }
  function sourceLabel(key: string) {
    const label = SOURCES.find((source) => source.key === key)?.label;
    return label ? t(label) : key;
  }
  function isAuto(row: Business, field: SuggestionField) {
    return Boolean(
      row.suggestions?.some((s) => s.field === field && s.auto && s.accepted)
    );
  }
  // Returns an English label; callers translate it with t().
  function rowVerdict(row: Business): { label: string; badge: string } {
    if (isQueued(row)) return { label: 'Verifying…', badge: 'running-badge' };
    const verdict = row.verification?.verdict;
    if (verdict === 'likely_active')
      return { label: 'Likely active', badge: 'reviewed-badge' };
    if (verdict === 'likely_ceased')
      return { label: 'Likely ceased', badge: 'ceased-badge' };
    if (verdict === 'unclear')
      return { label: 'Unclear', badge: 'attention-badge' };
    return { label: 'Not verified', badge: 'neutral-badge' };
  }
  function acceptable(suggestion: Suggestion) {
    return (
      !suggestion.accepted &&
      (suggestion.kind === 'new' || suggestion.kind === 'different') &&
      suggestion.field !== 'status' &&
      suggestion.field !== 'activity'
    );
  }
  function patch(body: unknown): Promise<Job> {
    if (!job) return Promise.reject(new Error('No task is open.'));
    return api('jobs/' + job.id, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
  }
  function syncEditor(data: Job, fields: SuggestionField[] = []) {
    if (!editor) return;
    const record = data.records.find((row) => row.id === editor?.id);
    if (!record) return;
    editor.verification = record.verification;
    editor.suggestions = record.suggestions;
    for (const field of fields) {
      if (field === 'activity') continue;
      editor[field] = record[field];
    }
    if (fields.length) editor.notes = record.notes;
  }
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
      .then((data) => {
        config = {
          ...data,
          sources: data.sources ?? {},
          defaultBudgetEur: data.defaultBudgetEur ?? 5
        };
        budget = config.defaultBudgetEur;
        const chosen: Record<string, boolean> = {};
        for (const source of SOURCES)
          chosen[source.key] =
            source.preset && Boolean(config.sources[source.key]);
        selectedSources = chosen;
      })
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
      if (enrich) {
        form.append('budget', String(budget));
        form.append(
          'sources',
          SOURCES.filter(
            (source) =>
              selectedSources[source.key] && config.sources[source.key]
          )
            .map((source) => source.key)
            .join(',')
        );
      }
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
      syncEditor(data);
      if (data.state === 'done') {
        screen = 'results';
        error = '';
        if (
          ['pending', 'sending'].includes(data.notification) ||
          data.records.some(isQueued)
        )
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
    industry = '';
    category = '';
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
  async function acceptSuggestion(suggestion: Suggestion) {
    if (!job || !editor || accepting) return;
    accepting = suggestion.field;
    editorError = '';
    try {
      const data = await patch({
        accept: { id: editor.id, field: suggestion.field }
      });
      job = data;
      syncEditor(data, [suggestion.field]);
    } catch (e) {
      editorError = (e as Error).message;
    } finally {
      accepting = '';
    }
  }
  async function acceptAllPending() {
    if (!job || !editor || accepting) return;
    editorError = '';
    try {
      for (const suggestion of editorPending) {
        if (!editor) break;
        accepting = suggestion.field;
        const data = await patch({
          accept: { id: editor.id, field: suggestion.field }
        });
        job = data;
        syncEditor(data, [suggestion.field]);
      }
    } catch (e) {
      editorError = (e as Error).message;
    } finally {
      accepting = '';
    }
  }
  async function undoSuggestion(suggestion: Suggestion) {
    if (!job || !editor || accepting) return;
    accepting = suggestion.field;
    editorError = '';
    try {
      const data = await patch({
        undo: { id: editor.id, field: suggestion.field }
      });
      job = data;
      syncEditor(data, [suggestion.field]);
    } catch (e) {
      editorError = (e as Error).message;
    } finally {
      accepting = '';
    }
  }
  async function verifyBusiness() {
    if (!job || !editor || verifying || editorQueued) return;
    verifying = true;
    editorError = '';
    try {
      const data = await patch({ verify: { id: editor.id } });
      job = data;
      syncEditor(data);
      clearTimeout(timer);
      timer = setTimeout(() => poll(data.id), 1500);
    } catch (e) {
      editorError = (e as Error).message;
    } finally {
      verifying = false;
    }
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
              'Real-world checks against websites, Google Maps and directories produce suggestions only. They do not verify legal registration.'
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
      <div class="upload-layout">
        <section class="upload-panel" aria-label={t('Upload a business file')}>
          <div class="section-heading">
            <h2 bind:this={heading} tabindex="-1">
              {t('Start with your file')}
            </h2>
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
                disabled={!anySourceReady}
              /><span
                ><strong>{t('Verify against real-world sources')}</strong><span
                  >{t(
                    'Check whether each business still exists and collect suggested corrections for review.'
                  )}</span
                ></span
              ></label
            ><span
              class="badge connection-badge"
              class:connected={anySourceReady}
              >{anySourceReady ? t('Connected') : t('Not connected')}</span
            >
            {#if enrich && anySourceReady}
              <div class="enrich-options">
                <span class="small-label">{t('SOURCES')}</span>
                <div class="source-grid">
                  {#each SOURCES as source (source.key)}
                    {@const ready = Boolean(config.sources[source.key])}
                    <label class="source-option" class:unavailable={!ready}
                      ><input
                        type="checkbox"
                        class="checkbox checkbox-xs"
                        disabled={!ready}
                        bind:checked={selectedSources[source.key]}
                      /><span
                        >{t(source.label)}{#if !ready}<span
                            class="badge connection-badge"
                            >{t('Not connected')}</span
                          >{/if}</span
                      ></label
                    >
                  {/each}
                </div>
                {#if selectedSources.companyweb_demo}
                  <p>
                    {t(
                      'Prepared Coja example only; no live Companyweb connection.'
                    )}
                    <a href="/companyweb-demo.csv" download
                      >{t('Download demo file')}</a
                    >
                  </p>
                {/if}
                <details class="advanced-details">
                  <summary>{t('Advanced')}</summary>
                  <label class="field-label budget-field" for="upload-budget"
                    >{t('Budget (EUR)')}</label
                  >
                  <div class="budget-row">
                    <input
                      id="upload-budget"
                      type="number"
                      class="input"
                      min="0.5"
                      step="0.5"
                      bind:value={budget}
                    />
                    <p>{t('Stop verifying when this estimate is reached.')}</p>
                  </div>
                </details>
              </div>
            {/if}
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
            >{job?.phase === 'enrich'
              ? t('Verifying against real-world sources')
              : job?.phase === 'google'
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
        {#if job?.enrich}
          <div class="verify-board" aria-label={t('Live verification')}>
            <div class="tally-strip" role="status" aria-live="polite">
              <span class="tally-pill active"
                >{t('Likely active')}
                <strong>{number(tallies.likely_active)}</strong></span
              ><span class="tally-pill ceased"
                >{t('Likely ceased')}
                <strong>{number(tallies.likely_ceased)}</strong></span
              ><span class="tally-pill unclear"
                >{t('Unclear')} <strong>{number(tallies.unclear)}</strong></span
              ><span class="tally-pill"
                >{t('Skipped')} <strong>{number(tallies.skipped)}</strong></span
              ><span class="tally-pill running"
                >{t('Running')} <strong>{number(tallies.running)}</strong></span
              ><span class="tally-pill"
                >{t('Waiting')} <strong>{number(tallies.waiting)}</strong></span
              >
            </div>
            <div class="board-scroll">
              {#each board as item (item.row.id)}
                {@const v = item.row.verification}
                <div class="board-row" class:running={v?.verdict === 'running'}>
                  <span class="board-name" title={item.row.name}
                    >{item.row.name || t('Unnamed business')}</span
                  >
                  <span class="chip-strip">
                    {#each jobStages as stage (stage.key)}
                      {@const state = chipState(item.row, stage.key)}
                      <span
                        class="stage-chip {state}"
                        title={chipTitle(item.row, stage.key)}
                        >{t(stage.label)}</span
                      >
                    {/each}
                  </span>
                  {#if v}<span
                      class="badge board-badge {VERDICT_BADGE[v.verdict] ??
                        'neutral-badge'}"
                      >{t(VERDICTS[v.verdict] ?? v.verdict)}</span
                    >{:else}<span class="board-waiting">{t('Waiting')}</span
                    >{/if}
                </div>
              {/each}
            </div>
            {#if rows.length > BOARD_LIMIT}<p class="board-more">
                {t('and {count} more', {
                  count: number(rows.length - BOARD_LIMIT)
                })}
              </p>{/if}
          </div>
        {:else}
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
              <span class="layer-status muted"
                ><Icon name="pin" size={16} /></span
              >
              <div>
                <strong>{t('Verify against real-world sources')}</strong>
                <p>
                  {job?.enrich
                    ? t(
                        'Websites, Google Maps and directories supply evidence; every suggestion is reviewed by you'
                      )
                    : t('Real-world verification was not requested')}
                </p>
              </div>
              <span>{job?.enrich ? t('Queued') : t('Skipped')}</span>
            </div>
          </div>
        {/if}
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
                ? t(
                    'Well-supported contact details were filled in automatically; everything else waits for your decision.'
                  )
                : t(
                    'Source data checked. Real-world verification was not run.'
                  )}</span
            >
            {#if job.enrichment}<span class="tally-strip summary-tallies"
                ><span class="tally-pill active"
                  >{t('Likely active')}
                  <strong>{number(tallies.likely_active)}</strong></span
                ><span class="tally-pill ceased"
                  >{t('Likely ceased')}
                  <strong>{number(tallies.likely_ceased)}</strong></span
                ><span class="tally-pill unclear"
                  >{t('Unclear')}
                  <strong>{number(tallies.unclear)}</strong></span
                ><span class="tally-pill"
                  >{t('Skipped')}
                  <strong>{number(tallies.skipped)}</strong></span
                ></span
              >{/if}
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
        <div
          class="universal-search"
          role="search"
          aria-label={t('Search this file')}
        >
          <label class="field-label" for="business-search"
            >{t('Find businesses')}</label
          >
          <div class="search-controls">
            <div class="search-input">
              <Icon name="search" size={20} />
              <input
                id="business-search"
                type="search"
                aria-label={t('Search businesses')}
                aria-describedby="search-help"
                bind:value={query}
                placeholder={t('Industry, name, location or keyword…')}
                onkeydown={(event) => {
                  if (event.key === 'Escape') query = '';
                }}
              />
              {#if query}<button
                  class="icon-button"
                  aria-label={t('Clear search')}
                  onclick={() => {
                    query = '';
                    document.getElementById('business-search')?.focus();
                  }}><Icon name="close" size={16} /></button
                >{/if}
            </div>
            <select
              class="select industry-select"
              aria-label={t('Filter by industry')}
              bind:value={industry}
              disabled={!industryOptions.length}
            >
              <option value="">{t('All industries')}</option>
              {#each industryOptions as option}<option value={option}
                  >{option}</option
                >{/each}
            </select>
            <select
              class="select category-select"
              aria-label={t('Filter by category')}
              bind:value={category}
              disabled={!categoryOptions.length}
            >
              <option value="">{t('All categories')}</option>
              {#each categoryOptions as option}<option value={option}
                  >{option}</option
                >{/each}
            </select>
          </div>
          <p id="search-help">
            {t(
              'Search across this file, including contact details, notes and original fields. Combine keywords to narrow results.'
            )}
          </p>
          {#if !industryOptions.length}<p>
              {t(
                'No industry fields in this file. Other keywords are still searchable.'
              )}
            </p>{/if}
          {#if !categoryOptions.length}<p>
              {t(
                'No category fields in this file. Add a category column to filter by category.'
              )}
            </p>{/if}
          <div class="search-feedback">
            <span role="status" aria-live="polite"
              >{t('{count} of {total} records match', {
                count: number(filtered.length),
                total: number(rows.length)
              })}</span
            >
            {#if query || industry || category || filter !== 'all'}<button
                class="review-button"
                onclick={() => {
                  query = '';
                  industry = '';
                  category = '';
                  filter = 'all';
                }}>{t('Reset search and filters')}</button
              >{/if}
          </div>
        </div>
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
              class:active={filter === 'ceased'}
              onclick={() => (filter = 'ceased')}
              >{t('Likely ceased')}
              <span class="brick-count">{number(ceased)}</span></button
            ><button
              class:active={filter === 'missing'}
              onclick={() => (filter = 'missing')}
              >{t('Missing contact info')}</button
            >
          </div>
          <div class="toolbar-right">
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
                >{#each visible as row}{@const verdict =
                    rowVerdict(row)}{@const auto =
                    autoVerified(row).length}{@const pending =
                    pendingSuggestions(row).length}<tr
                    ><td
                      ><div class="name-cell"
                        >{#if row.mergedFrom?.length}<button
                            class="group-toggle"
                            aria-expanded={expandedGroups.has(row.id)}
                            aria-label={expandedGroups.has(row.id)
                              ? t('Hide merged records')
                              : t('Show merged records')}
                            onclick={() => toggleGroup(row.id)}
                            ><Icon name="chevron" size={12} /></button
                          >{/if}<strong
                          >{row.name || t('Unnamed business')}</strong
                        ></div
                      ><span class="row-secondary"
                        >{row.number || t('No identifier')}
                        <span class="dot-divider">·</span>
                        {row.kind === 'establishment'
                          ? t('Establishment')
                          : t('Enterprise')}</span
                      >{#if industries(row.source).length}<span
                          class="row-secondary industry-label"
                          >{industries(row.source).join(' · ')}</span
                        >{/if}{#if categories(row.source).length}<span
                          class="row-secondary industry-label"
                          >{t('Category')}: {categories(row.source).join(
                            ' · '
                          )}</span
                        >{/if}{#if row.mergedFrom?.length}<span
                          class="badge merged-badge"
                          >{row.mergedFrom.length === 1
                            ? t('1 merged')
                            : t('{count} merged', {
                                count: number(row.mergedFrom.length)
                              })}</span
                        >{/if}</td
                    ><td class="address-cell"
                      >{row.address ||
                        t(
                          'No address provided'
                        )}{#if row.phone || row.email}<span class="row-contact"
                          >{#if row.phone}<span
                              >{row.phone}{#if isAuto(row, 'phone')}<span
                                  class="auto-tag">{t('auto')}</span
                                >{/if}</span
                            >{/if}{#if row.email}<span
                              >{row.email}{#if isAuto(row, 'email')}<span
                                  class="auto-tag">{t('auto')}</span
                                >{/if}</span
                            >{/if}</span
                        >{/if}</td
                    ><td
                      ><span class="source-status"
                        >{row.status || t('Not provided')}</span
                      ></td
                    ><td
                      >{#if row.reviewed}<span class="badge reviewed-badge"
                          ><Icon name="check" size={12} />{t('Reviewed')}</span
                        >{:else}<span class="badge {verdict.badge}"
                          >{t(verdict.label)}</span
                        >{/if}{#if auto || pending || row.issues[0] || row.googleError}<span
                          class="row-summary"
                          >{#if auto}<span class="mini-pill auto"
                              ><Icon name="check" size={10} />{t(
                                '{count} auto-verified',
                                { count: number(auto) }
                              )}</span
                            >{/if}{#if pending}<span class="mini-pill pending"
                              >{t('{count} to decide', {
                                count: number(pending)
                              })}</span
                            >{/if}{#if row.issues[0]}<span class="row-issue"
                              >{message(row.issues[0])}</span
                            >{:else if row.googleError}<span class="row-issue"
                              >{t('Google Maps unavailable')}</span
                            >{/if}</span
                        >{/if}</td
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
                  >{#if row.mergedFrom?.length && expandedGroups.has(row.id)}{#each mergedByCanonical.get(row.id) ?? [] as sub}<tr
                        class="sub-row"
                        ><td
                          ><span class="sub-thread" aria-hidden="true"
                          ></span><strong
                            >{sub.name || t('Unnamed business')}</strong
                          ><span class="row-secondary"
                            >{sub.number || t('No identifier')}</span
                          ></td
                        ><td class="address-cell"
                          >{sub.address || t('No address provided')}</td
                        ><td
                          ><span class="row-secondary"
                            >{t('Merged into {name}', {
                              name: row.name || t('Unnamed business')
                            })}</span
                          ></td
                        ><td></td><td></td></tr
                      >{/each}{/if}{/each}</tbody
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
                    industry = '';
                    category = '';
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
      <div class="editor-header-actions">
        <button
          class="btn btn-outline btn-sm"
          type="button"
          disabled={saving || verifying || editorQueued || !anySourceReady}
          title={anySourceReady
            ? t('Re-run real-world verification for this record')
            : t('No verification source is connected')}
          onclick={verifyBusiness}
          >{#if verifying || editorQueued}<span
              class="loading loading-spinner loading-xs"
            ></span>{editorQueued ? t('Verifying…') : t('Queuing…')}{:else}{t(
              'Verify this business'
            )}{/if}</button
        ><button
          class="icon-button"
          disabled={saving}
          onclick={() => (editor = null)}
          data-close-record
          aria-label={t('Close record')}><Icon name="close" /></button
        >
      </div>
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
      {#if editor.verification}
        {@const v = editor.verification}
        <div class="verification-block">
          <div
            class="verdict-banner"
            class:active={v.verdict === 'likely_active'}
            class:ceased={v.verdict === 'likely_ceased'}
            class:unclear={v.verdict === 'unclear'}
            class:queued={v.verdict === 'not_run' || v.verdict === 'running'}
            role="status"
          >
            <div class="verdict-line">
              <strong
                >{#if isQueued(editor)}<span
                    class="loading loading-spinner loading-xs"
                  ></span>{/if}{t(VERDICTS[v.verdict] ?? v.verdict)}</strong
              >
            </div>
            {#if isQueued(editor)}
              <p>
                {v.verdict === 'running'
                  ? t(
                      'Sources are being checked. The trail below fills in as each stage finishes.'
                    )
                  : t(
                      'Waiting for a free worker. Verification starts in a moment.'
                    )}
              </p>
            {:else}
              {#if v.reason}<p>{message(v.reason)}</p>{/if}
              <span class="verdict-meta"
                >{v.sources?.length
                  ? t('Sources: {sources}', {
                      sources: v.sources.map(sourceLabel).join(', ')
                    })
                  : v.skipped
                    ? t('Skipped: {reason}', {
                        reason: v.skipped.replaceAll('_', ' ')
                      })
                    : t('No sources ran')}</span
              >
            {/if}
          </div>
          {#if v.trace?.length}
            <details class="trail-details" open>
              <summary>{t('Verification trail')}</summary>
              <ol class="trail">
                {#each v.trace as step, i (i)}
                  <li class="trail-step {step.status}">
                    {#if step.status === 'running'}<span
                        class="loading loading-spinner loading-xs trail-dot"
                      ></span>{:else}<span class="trail-dot"></span>{/if}
                    <span class="trail-stage"
                      >{t(STAGE_LABELS[step.stage] ?? step.stage)}</span
                    >
                    <span class="trail-note"
                      >{message(step.note)}
                      {#if step.demo}
                        <span class="companyweb-demo-preview">
                          <strong
                            >{t('Prepared snapshot: {date}', {
                              date: step.demo.capturedAt
                            })} · {step.demo.enterprise}</strong
                          >
                          <span
                            >{t('Company: {name}', {
                              name: step.demo.name
                            })}</span
                          >
                          <span
                            >{t('Registered address: {address}', {
                              address: step.demo.address
                            })}</span
                          >
                          <span
                            >{t('Status in snapshot: {status}', {
                              status: step.demo.status
                            })}</span
                          >
                          <span
                            >{t(
                              'Uploaded record: {name} · {address} · {status}',
                              {
                                name: editor.name,
                                address: editor.address,
                                status: editor.status
                              }
                            )}</span
                          >
                          {#if editor.kind === 'establishment'}<span
                              >{t(
                                'This snapshot describes the parent enterprise, not this establishment.'
                              )}</span
                            >{/if}
                          <a
                            href={step.demo.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            >{t('View Companyweb page')}
                            <Icon name="external" size={12} /></a
                          >
                        </span>
                      {/if}
                    </span>
                    <span class="trail-meta"
                      >{#if step.findings > 0}<span
                          >{t('{count} found', {
                            count: number(step.findings)
                          })}</span
                        >{/if}<span>{duration(step)}</span></span
                    >
                  </li>
                {/each}
              </ol>
            </details>
          {/if}
          {#if editor.suggestions?.length && !isQueued(editor)}
            {#if editorPending.length >= 2}
              <div class="suggestion-toolbar">
                <span
                  >{t('{count} suggestions waiting', {
                    count: number(editorPending.length)
                  })}</span
                >
                <button
                  type="button"
                  class="btn btn-outline btn-sm"
                  disabled={Boolean(accepting) || saving}
                  onclick={acceptAllPending}
                  >{accepting
                    ? t('Accepting…')
                    : t('Accept all pending')}</button
                >
              </div>
            {/if}
            <div class="suggestion-scroll">
              <table class="suggestion-table">
                <thead
                  ><tr
                    ><th>{t('Field')}</th><th>{t('Suggested')}</th><th
                      >{t('Confidence')}</th
                    ><th>{t('Source')}</th><th
                      ><span class="sr-only">{t('Action')}</span></th
                    ></tr
                  ></thead
                >
                <tbody>
                  {#each editor.suggestions as suggestion (suggestion.field)}
                    {@const first = suggestion.evidence?.[0]}
                    {@const pct = Math.round(
                      Math.min(1, Math.max(0, suggestion.confidence)) * 100
                    )}
                    <tr>
                      <td
                        ><strong>{t(FIELD_LABELS[suggestion.field])}</strong
                        ></td
                      >
                      <td class="suggestion-value"
                        >{suggestion.value ||
                          '—'}{#if suggestion.kind === 'different' && suggestion.current}<span
                            class="suggestion-current"
                            title={t('Value in the uploaded file')}
                            >{t('was {value}', {
                              value: suggestion.current
                            })}</span
                          >{/if}</td
                      >
                      <td
                        ><span class="confidence"
                          ><span class="confidence-bar"
                            ><i style:width="{pct}%"></i></span
                          >{pct}%</span
                        ></td
                      >
                      <td
                        >{#if first?.url?.startsWith('http')}<a
                            href={first.url}
                            target="_blank"
                            rel="noreferrer"
                            title={first.note ? message(first.note) : undefined}
                            >{sourceLabel(first.source)}<Icon
                              name="external"
                              size={12}
                            /></a
                          >{:else if first}<span
                            title={first.note ? message(first.note) : undefined}
                            >{sourceLabel(first.source)}</span
                          >{:else}—{/if}</td
                      >
                      <td
                        >{#if suggestion.auto && suggestion.accepted}<span
                            class="suggestion-actions"
                            ><span class="badge reviewed-badge"
                              ><Icon name="check" size={11} />{t(
                                'Auto-verified'
                              )}</span
                            ><button
                              type="button"
                              class="btn btn-ghost btn-sm"
                              disabled={Boolean(accepting) || saving}
                              onclick={() => undoSuggestion(suggestion)}
                              >{accepting === suggestion.field
                                ? t('Undoing…')
                                : t('Undo')}</button
                            ></span
                          >{:else}<button
                            type="button"
                            class="btn btn-outline btn-sm"
                            disabled={suggestion.accepted ||
                              suggestion.kind === 'confirmed' ||
                              Boolean(accepting) ||
                              saving}
                            onclick={() => acceptSuggestion(suggestion)}
                            >{suggestion.accepted
                              ? t('Accepted')
                              : suggestion.kind === 'confirmed'
                                ? t('Matches')
                                : accepting === suggestion.field
                                  ? t('Accepting…')
                                  : t('Accept')}</button
                          >{/if}</td
                      >
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>
      {/if}
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
            <dt>{t('Industry')}</dt>
            <dd>{placeIndustry(editor.google) || t('Not provided')}</dd>
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
