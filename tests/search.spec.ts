import { test, expect } from '@playwright/test';
import {
  indexBusiness,
  matchesSearch,
  industries,
  categories
} from '../src/lib/search';
import type { Business } from '../src/lib/types';

test('indexes nested industry fields and matches accent-insensitive terms across fields', () => {
  expect(
    industries({
      activities: [{ description: 'Café', nace: '56.30' }],
      Sector: 'Retail; Food'
    })
  ).toEqual(['Café', '56.30', 'Retail', 'Food']);
  expect(
    categories({
      metadata: { categorie: ['Bakery', 'Coffee'] },
      business_category: 'Retail; Food'
    })
  ).toEqual(['Bakery', 'Coffee', 'Retail', 'Food']);
  expect(categories({ industry: 'Retail' })).toEqual([]);
  const entry = indexBusiness({
    name: 'Élan',
    address: 'Bruxelles',
    notes: 'Wholesale',
    source: { industry: 'Food', custom: ['Organic'] }
  } as unknown as Business);
  expect(matchesSearch(entry.text, '  CAFE  ')).toBe(false);
  expect(matchesSearch(entry.text, 'organic elan food')).toBe(true);
  expect(matchesSearch(entry.text, 'food antwerp')).toBe(false);
  expect(matchesSearch(entry.text, '   ')).toBe(true);
});

test('KBO activity codes and descriptions are filterable without version or count metadata', () => {
  const source = {
    NACE_hoofdact_BTW: '71.111',
    NACE_versie_BTW: '2008',
    Omschrijving_hoofdact_BTW: 'Architecten',
    Aantal_hoofdact_BTW: 1,
    NACE_hoofdact_RSZ: '71111',
    NACE_Versie_RSZ: '2025',
    Omschrijving_hoofdact_RSZ: 'Bouwarchitecten',
    Aantal_Hoofdact_RSZ: 1
  };
  expect(industries(source)).toEqual([
    '71.111',
    'Architecten',
    '71111',
    'Bouwarchitecten'
  ]);
  expect(categories(source)).toEqual(industries(source));
  const entry = indexBusiness({
    source,
    name: 'Bureau Nicolas'
  } as unknown as Business);
  expect(matchesSearch(entry.text, 'architecten nicolas')).toBe(true);
  expect(matchesSearch(entry.text, '71111')).toBe(true);
  const dottedOnly = indexBusiness({
    source: { NACE_hoofdact_BTW: '71.111' }
  } as unknown as Business);
  expect(matchesSearch(dottedOnly.text, '71111')).toBe(true);
  expect(
    categories({
      NACE_hoofdact_BTW: ' ',
      Omschrijving_hoofdact_BTW: '',
      Aantal_hoofdact_BTW: 0
    })
  ).toEqual([]);
});

for (const locale of ['en', 'nl']) {
  test(`universal search and industry filtering in ${locale}`, async ({
    page,
    context,
    baseURL
  }) => {
    await context.addCookies([
      { name: 'kbo-language', value: locale, url: baseURL! }
    ]);
    await page.goto('/');
    await expect(page.locator('.dropzone')).toBeEnabled();
    await page.locator('input[type=file]').setInputFiles({
      name: 'industries.json',
      mimeType: 'application/json',
      buffer: Buffer.from(
        JSON.stringify([
          {
            name: 'Café Élan',
            number: '0123456749',
            address: 'Brussels',
            industry: 'Hospitality',
            category: 'Coffee',
            tags: ['Organic'],
            email: 'hello@elan.example'
          },
          {
            name: 'City Shop',
            number: '0123456848',
            address: 'Antwerp',
            sector: 'Retail',
            categorie: ['Groceries'],
            tags: ['Organic']
          },
          {
            name: 'Canal Café',
            number: '0123456947',
            address: 'Ghent',
            industry: 'Hospitality',
            categories: ['Bakery', 'Coffee']
          }
        ])
      )
    });
    await page
      .getByRole('button', {
        name: locale === 'en' ? 'Organise file' : 'Bestand ordenen',
        exact: true
      })
      .click();
    await expect(page.locator('tbody tr')).toHaveCount(3);
    const input = page.locator('#business-search');
    await page.locator('.category-select').selectOption('Coffee');
    await page.locator('.industry-select').selectOption('Hospitality');
    await expect(page.locator('tbody tr')).toHaveCount(2);
    await input.fill('brussels');
    await expect(page.locator('tbody tr')).toHaveCount(1);
    await expect(page.locator('tbody')).toContainText('Café Élan');
    await page.locator('.category-select').selectOption('Groceries');
    await expect(page.locator('tbody tr')).toHaveCount(0);
    await page
      .getByRole('button', {
        name:
          locale === 'en'
            ? 'Reset search and filters'
            : 'Zoekopdracht en filters wissen'
      })
      .click();
    await expect(page.locator('.category-select')).toHaveValue('');
    await expect(page.locator('.industry-select')).toHaveValue('');
    await expect(input).toHaveValue('');
    await expect(page.locator('tbody tr')).toHaveCount(3);

    await input.fill('cafe brussels');
    await expect(page.locator('tbody tr')).toHaveCount(1);
    await expect(page.locator('tbody')).toContainText('Café Élan');
    await input.fill('organic');
    await expect(page.locator('tbody tr')).toHaveCount(2);
    await page.locator('.industry-select').selectOption('Retail');
    await expect(page.locator('tbody tr')).toHaveCount(1);
    await expect(page.locator('tbody')).toContainText('City Shop');
    await input.fill('hospitality');
    await expect(page.locator('tbody tr')).toHaveCount(0);
    await page
      .getByRole('button', {
        name: locale === 'en' ? 'Show all records' : 'Alle records tonen',
        exact: true
      })
      .click();
    await expect(page.locator('tbody tr')).toHaveCount(3);
    await expect(page.locator('.industry-select')).toHaveValue('');
    await input.fill('hello@elan.example');
    await expect(page.locator('tbody tr')).toHaveCount(1);
    await input.press('Escape');
    await expect(page.locator('tbody tr')).toHaveCount(3);
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(input).toBeVisible();
    await expect(page.locator('.category-select')).toBeVisible();
    await expect(page.locator('.industry-select')).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth
      )
    ).toBe(true);
    await page.screenshot({
      path: `.context/search-${locale}-mobile.png`,
      fullPage: true
    });
  });
}
