import { test, expect } from '@playwright/test';
import { readFile } from 'node:fs/promises';

test('Dutch is server-rendered, the mobile switcher persists, and validation follows the language', async ({
  page,
  request
}) => {
  const response = await request.get('/');
  const html = await response.text();
  expect(html).toContain('lang="nl"');
  expect(html).toContain('Goede beslissingen beginnen');
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');
  await expect(page.locator('.dropzone')).toBeEnabled();
  await expect(page.getByRole('combobox', { name: 'Taal' })).toBeVisible();
  await page
    .locator('input[type=file]')
    .setInputFiles({
      name: 'empty.csv',
      mimeType: 'text/csv',
      buffer: Buffer.from('')
    });
  await expect(page.getByRole('alert')).toContainText('Dit bestand is leeg.');
  await page.getByRole('combobox', { name: 'Taal' }).selectOption('en');
  await expect(page.getByRole('alert')).toContainText('This file is empty.');
  await expect(page.locator('html')).toHaveAttribute('lang', 'en');
  await page.reload();
  await expect(
    page.getByRole('heading', { name: /Good decisions/ })
  ).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'Language' })).toHaveValue(
    'en'
  );
  await page.getByRole('combobox', { name: 'Language' }).selectOption('nl');
  await page.reload();
  await expect(
    page.getByRole('heading', { name: /Goede beslissingen/ })
  ).toBeVisible();
  await page.getByRole('button', { name: 'Zo werkt het' }).click();
  await expect(
    page.getByText('Van export naar bruikbare gegevens')
  ).toBeVisible();
  await page.getByRole('button', { name: 'Help sluiten' }).click();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth
    )
  ).toBe(true);
  await page.screenshot({ path: '.context/dutch-mobile.png', fullPage: true });
});

test('Dutch upload, backend errors, review, language switching, and unchanged exports', async ({
  page
}) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.goto('/');
  await expect(page.locator('.dropzone')).toBeEnabled();
  await page
    .locator('input[type=file]')
    .setInputFiles({
      name: 'bad.csv',
      mimeType: 'text/csv',
      buffer: Buffer.from('name,name\none,two')
    });
  await page
    .getByRole('button', { name: 'Bestand ordenen', exact: true })
    .click();
  await expect(page.getByRole('alert')).toContainText(
    'CSV-kolomnamen mogen niet leeg zijn en moeten uniek zijn'
  );
  await page.locator('input[type=file]').setInputFiles('static/example.csv');
  await page
    .getByRole('button', { name: 'Bestand ordenen', exact: true })
    .click();
  await expect(
    page.getByRole('heading', { name: 'Klaar om na te kijken.' })
  ).toBeVisible();
  await expect(page.locator('tbody tr')).toHaveCount(3);
  await expect(page.getByText('Dubbel nummer').first()).toBeVisible();
  const jobURL = page.url();
  await page
    .getByRole('button', { name: 'Voorbeeld Atelier nakijken', exact: true })
    .first()
    .click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).not.toBeVisible();
  await page
    .getByRole('button', { name: 'Voorbeeld Atelier nakijken', exact: true })
    .first()
    .click();
  await page
    .getByLabel('Bedrijfsnaam', { exact: true })
    .fill('Nagekeken voorbeeld');
  await page
    .getByLabel('Notities bij de controle')
    .fill('Gecontroleerd in de oorspronkelijke export.');
  await page.getByLabel('Ik heb dit record nagekeken').check();
  await page.getByRole('button', { name: 'Wijzigingen opslaan' }).click();
  await expect(page.getByRole('dialog')).not.toBeVisible();
  await page.getByRole('combobox', { name: 'Taal' }).selectOption('en');
  await expect(
    page.getByRole('heading', { name: 'Ready for a closer look.' })
  ).toBeVisible();
  expect(page.url()).toBe(jobURL);
  await expect(
    page.getByText('Nagekeken voorbeeld', { exact: true })
  ).toBeVisible();
  await page.getByRole('combobox', { name: 'Language' }).selectOption('nl');
  await page.reload();
  await expect(
    page.getByText('Nagekeken voorbeeld', { exact: true })
  ).toBeVisible();
  await page.getByLabel('Bedrijven zoeken').fill('bestaat niet');
  await expect(page.getByText('Geen records gevonden')).toBeVisible();
  await page.getByRole('button', { name: 'Alle records tonen' }).click();
  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: 'JSON downloaden' }).click();
  const result = JSON.parse(
    await readFile((await (await download).path())!, 'utf8')
  );
  expect(result.records[0].name).toBe('Nagekeken voorbeeld');
  expect(result.records[0].source.Maatschappelijke_naam).toBe(
    'Voorbeeld Atelier'
  );
  expect(result.records[0].issues).toContain('Duplicate identifier');
  await page.screenshot({ path: '.context/dutch-results.png', fullPage: true });
  expect(errors).toEqual([]);
});

test('processing, notification states, and parameterized service messages are Dutch', async ({
  page
}) => {
  const id = 'a'.repeat(48);
  await page.route('**/api/jobs/' + id, (route) =>
    route.fulfill({
      json: {
        id,
        state: 'processing',
        phase: 'google',
        filename: 'voorbeeld.csv',
        total: 2000,
        progress: 1500,
        records: [],
        email: 'test@example.be',
        enrich: true,
        notification: 'pending'
      }
    })
  );
  await page.goto('/?job=' + id);
  await expect(
    page.getByRole('heading', { name: 'We brengen alles op orde.' })
  ).toBeVisible();
  await expect(
    page.getByText('1.500 van 2.000 records verwerkt')
  ).toBeVisible();
  await expect(
    page.getByText('Suggesties uit Google Maps vergelijken')
  ).toBeVisible();
  await expect(
    page.getByText('Een e-mailmelding is aangevraagd voor test@example.be')
  ).toBeVisible();
  await page.unroute('**/api/jobs/' + id);
  await page.route('**/api/jobs/' + id, (route) =>
    route.fulfill({
      status: 400,
      json: {
        error: 'CSV row 4 is malformed: check its column count and quotes'
      }
    })
  );
  await expect(page.getByRole('alert')).toContainText('CSV-rij 4 is ongeldig');
});
