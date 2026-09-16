import { test, expect } from '@playwright/test';

for (const locale of ['en', 'nl']) {
  test(`Companyweb fixture is clearly simulated in ${locale}`, async ({
    page,
    context,
    baseURL
  }) => {
    await context.addCookies([
      { name: 'kbo-language', value: locale, url: baseURL! }
    ]);
    await page.goto('/');
    await expect(page.locator('.dropzone')).toBeEnabled();
    await page
      .locator('input[type=file]')
      .setInputFiles('static/companyweb-demo.csv');
    await page
      .getByRole('checkbox', {
        name:
          locale === 'en'
            ? /Verify against real-world sources/
            : /Controleer tegen echte bronnen/
      })
      .check();
    for (const checkbox of await page
      .locator('.source-option input:not(:disabled)')
      .all())
      await checkbox.uncheck();
    await page.getByRole('checkbox', { name: 'Companyweb · Demo' }).check();
    await expect(
      page.getByRole('link', {
        name: locale === 'en' ? 'Download demo file' : 'Demobestand downloaden'
      })
    ).toHaveAttribute('href', '/companyweb-demo.csv');
    await page
      .getByRole('button', {
        name: locale === 'en' ? 'Organise file' : 'Bestand ordenen',
        exact: true
      })
      .click();
    await expect(page.locator('tbody tr')).toHaveCount(1);
    await page.locator('tbody tr').first().getByRole('button').click();
    const dialog = page.getByRole('dialog');
    await expect(dialog.locator('.companyweb-demo-preview')).toContainText(
      '0725606718'
    );
    await expect(dialog.locator('.companyweb-demo-preview')).toContainText(
      'Alfons Servaislei 53, 2900 Schoten'
    );
    await expect(
      dialog
        .locator('.trail-note')
        .filter({ has: page.locator('.companyweb-demo-preview') })
    ).toContainText(
      locale === 'en'
        ? 'No live Companyweb request'
        : 'Geen live Companyweb-verzoek'
    );
    await expect(
      dialog.getByRole('button', { name: /^(Accept|Accepteren)$/ })
    ).toHaveCount(0);
  });
}
