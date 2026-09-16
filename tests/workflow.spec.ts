import { test, expect } from '@playwright/test';
import { readFile } from 'node:fs/promises';
import { exportJob } from '../src/lib/export';
import type { Job } from '../src/lib/types';

// Keep the original English workflow covered alongside the Dutch default.
test.beforeEach(async ({ context, baseURL }) => {
  await context.addCookies([
    { name: 'kbo-language', value: 'en', url: baseURL! }
  ]);
});

test('CSV upload, review, durable edits, filtering, and JSON / CSV / GeoJSON downloads', async ({
  page
}) => {
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.setViewportSize({ width: 1440, height: 1100 });
  await page.goto('/');
  await expect(page.locator('.dropzone')).toBeEnabled();
  await expect(
    page.getByRole('heading', { name: /Good decisions/ })
  ).toBeVisible();
  await page.screenshot({
    path: '.context/upload-desktop.png',
    fullPage: true
  });
  await page.locator('input[type=file]').setInputFiles('static/example.csv');
  await expect(page.getByText('Ready to organise')).toBeVisible();
  await page
    .getByRole('button', { name: 'Organise file', exact: true })
    .click();
  await expect(
    page.getByRole('heading', { name: 'Ready for a closer look.' })
  ).toBeVisible();
  await expect(page.locator('tbody tr')).toHaveCount(3);
  await page.screenshot({
    path: '.context/results-desktop.png',
    fullPage: true
  });
  await page
    .getByRole('button', { name: 'Review Voorbeeld Atelier', exact: true })
    .first()
    .click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await page
    .getByLabel('Business name', { exact: true })
    .fill('Reviewed example');
  await page
    .getByLabel('Review notes')
    .fill('Confirmed against original export.');
  await page.getByLabel('I have reviewed this record').check();
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByRole('dialog')).not.toBeVisible();
  await page.reload();
  await expect(
    page.getByText('Reviewed example', { exact: true })
  ).toBeVisible();
  await page.getByLabel('Search businesses').fill('Reviewed example');
  await expect(page.locator('tbody tr')).toHaveCount(1);
  await page.getByLabel('Search businesses').fill('nothing like this');
  await expect(page.getByText('No records match this view')).toBeVisible();
  await page
    .getByRole('button', { name: 'Show all records', exact: true })
    .click();
  const dl = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Download JSON' }).click();
  const saved = await dl;
  const result = JSON.parse(await readFile((await saved.path())!, 'utf8'));
  expect(result.records).toHaveLength(3);
  expect(result.records[0].name).toBe('Reviewed example');
  expect(result.records[0].source.Maatschappelijke_naam).toBe(
    'Voorbeeld Atelier'
  );
  expect(result.records[0].source.Ondernemingsnr).toBe('0123456749');
  expect(result.records[0].geometry.coordinates).toEqual([4.5, 51.25]);
  for (const format of ['CSV', 'GeoJSON']) {
    if (
      !(await page
        .getByRole('button', { name: 'Download ' + format, exact: true })
        .isVisible())
    )
      await page.getByLabel('Other download formats').click();
    const download = page.waitForEvent('download');
    await page
      .getByRole('button', { name: 'Download ' + format, exact: true })
      .click();
    const contents = await readFile((await (await download).path())!, 'utf8');
    if (format === 'GeoJSON')
      expect(JSON.parse(contents).features).toHaveLength(3);
    else expect(contents).toContain('review_name');
  }
  expect(errors).toEqual([]);
});

test('mobile drag and drop, invalid files, and JSON / GeoJSON inputs', async ({
  page
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');
  await expect(page.locator('.dropzone')).toBeEnabled();
  await page.screenshot({ path: '.context/upload-mobile.png', fullPage: true });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth
    )
  ).toBe(true);
  await page.locator('input[type=file]').setInputFiles({
    name: 'bad.csv',
    mimeType: 'text/csv',
    buffer: Buffer.from('name,name\none,two')
  });
  await page
    .getByRole('button', { name: 'Organise file', exact: true })
    .click();
  await expect(page.getByRole('alert')).toContainText('column names');
  const records = [
    {
      number: '0123456749',
      name: 'Synthetic JSON',
      address: 'Example 1, Schoten',
      longitude: 4.5,
      latitude: 51.2
    }
  ];
  for (const format of ['json', 'geojson']) {
    await page.goto('/');
    await expect(page.locator('.dropzone')).toBeEnabled();
    const contents = JSON.stringify(
      format === 'json'
        ? records
        : {
            type: 'FeatureCollection',
            features: [
              {
                type: 'Feature',
                properties: records[0],
                geometry: { type: 'Point', coordinates: [4.5, 51.2] }
              }
            ]
          }
    );
    const dataTransfer = await page.evaluateHandle(
      ({ contents, format }) => {
        const dt = new DataTransfer();
        dt.items.add(
          new File([contents], 'example.' + format, {
            type: 'application/json'
          })
        );
        return dt;
      },
      { contents, format }
    );
    await page.locator('.dropzone').dispatchEvent('drop', { dataTransfer });
    await page
      .getByRole('button', { name: 'Organise file', exact: true })
      .click();
    await expect(
      page.getByRole('heading', { name: 'Ready for a closer look.' })
    ).toBeVisible();
    await expect(page.locator('tbody tr')).toHaveCount(1);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth
      )
    ).toBe(true);
  }
});

test('CSV export neutralises formulas and retains conflicting original column names', () => {
  const record = {
    source: { name: '=HYPERLINK("bad")', review_name: 'keep' },
    name: 'Changed',
    number: '0123456749',
    issues: [],
    geometry: null
  };
  const csv = exportJob(
    { filename: 'x.csv', records: [record] } as unknown as Job,
    'csv'
  );
  expect(csv).toContain("'=HYPERLINK");
  expect(csv).toContain('"review_name"');
  expect(csv).toContain('"_review_name"');
});
