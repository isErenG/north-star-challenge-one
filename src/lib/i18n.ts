export type Locale = 'nl' | 'en';
export const localeCookie = 'kbo-language';
export function resolveLocale(value: string | undefined): Locale {
  return value === 'en' ? 'en' : 'nl';
}
const nl: Record<string, string> = {
  'View as': 'Weergave',
  List: 'Lijst',
  Map: 'Kaart',
  'Show records on a map': 'Toon records op de kaart',
  'Set MAPBOX_ACCESS_TOKEN to enable the map view':
    'Stel MAPBOX_ACCESS_TOKEN in om de kaartweergave te activeren',
  'Marker colours': 'Kleuren van kaartmarkeringen',
  'Map of business locations': 'Kaart met bedrijfslocaties',
  'Map unavailable': 'Kaart niet beschikbaar',
  'Loading map…': 'Kaart wordt geladen…',
  'No records with coordinates in this view':
    'Geen records met coördinaten in deze weergave',
  'Try another filter or search.':
    'Probeer een ander filter of een andere zoekopdracht.',
  'The map could not be loaded. Check the Mapbox token.':
    'De kaart kon niet worden geladen. Controleer het Mapbox-token.',
  'The map could not be loaded.': 'De kaart kon niet worden geladen.',
  '{count} of {total} records have coordinates':
    '{count} van {total} records hebben coördinaten',
  '{count} without coordinates are only shown in the list':
    '{count} zonder coördinaten worden alleen in de lijst getoond',
  '1 without coordinates is only shown in the list':
    '1 zonder coördinaten wordt alleen in de lijst getoond',
  'Zoom in': 'Inzoomen',
  'Zoom out': 'Uitzoomen',
  'Reset bearing to north': 'Kaart naar het noorden draaien',
  'Use two fingers to move the map':
    'Gebruik twee vingers om de kaart te verplaatsen',
  'Use Ctrl + scroll to zoom the map':
    'Gebruik Ctrl + scrollen om de kaart te zoomen',
  'Use ⌘ + scroll to zoom the map':
    'Gebruik ⌘ + scrollen om de kaart te zoomen',
  '1 record needs a closer look.': '1 record moet worden nagekeken.',
  'Results could not be saved. Please try again.':
    'De resultaten konden niet worden opgeslagen. Probeer het opnieuw.',
  'KBO Review — Business data, in order':
    'KBO Review — Bedrijfsgegevens op orde',
  'Organise KBO business records, review data quality and export a clear, editable dataset.':
    'Orden je KBO-bedrijfsgegevens, controleer de datakwaliteit en exporteer een overzichtelijk, bewerkbaar bestand.',
  'KBO Review home': 'KBO Review startpagina',
  'Business data workspace': 'Werkruimte voor bedrijfsgegevens',
  'How it works': 'Zo werkt het',
  'Crossroads Bank for Enterprises': 'Kruispuntbank van Ondernemingen',
  'Data review': 'Gegevens controleren',
  'Independent workspace': 'Onafhankelijke werkruimte',
  'File review progress': 'Voortgang van de bestandscontrole',
  'Upload your file': 'Bestand uploaden',
  'Check & organise': 'Controleren en ordenen',
  'Review & export': 'Nakijken en exporteren',
  'From one export to a usable dataset': 'Van export naar bruikbare gegevens',
  'Upload an authorised KBO export. We organise its columns and flag missing data, duplicate identifiers and status checks. Review any corrections, then download your full dataset. Original source fields stay with every record.':
    'Upload een KBO-export die je mag verwerken. We ordenen de kolommen en signaleren ontbrekende gegevens, dubbele nummers en te controleren statussen. Kijk de wijzigingen na en download je volledige bestand. De oorspronkelijke velden blijven bij elk record bewaard.',
  'Google Maps lookups are optional suggestions. They do not verify legal registration or provide email addresses.':
    'Resultaten uit Google Maps zijn optionele suggesties. Ze bevestigen geen wettelijke registratie en bevatten geen e-mailadressen.',
  'Close help': 'Help sluiten',
  Retry: 'Opnieuw proberen',
  'Start again': 'Opnieuw beginnen',
  'Upload a business file': 'Een bestand met bedrijfsgegevens uploaden',
  'Start with your file': 'Begin met je bestand',
  'STEP 01': 'STAP 01',
  'Upload file': 'Bestand uploaden',
  'Please upload one file at a time.': 'Upload één bestand tegelijk.',
  'Ready to organise': 'Klaar om te ordenen',
  'Choose a different file': 'Kies een ander bestand',
  'Drop your business data here': 'Sleep je bedrijfsgegevens hierheen',
  or: 'of',
  'browse files': 'kies een bestand',
  'on your computer': 'op je computer',
  'Up to 10 MB': 'Tot 10 MB',
  'Want to see how it works?': 'Benieuwd hoe het werkt?',
  'Download an example': 'Download een voorbeeld',
  'Compare with Google Maps': 'Vergelijk met Google Maps',
  'Look for address, phone and business-status suggestions.':
    'Zoek suggesties voor adressen, telefoonnummers en bedrijfsstatussen.',
  Connected: 'Verbonden',
  'Not connected': 'Niet verbonden',
  'Get an email when it’s ready': 'Ontvang een e-mail zodra het klaar is',
  Optional: 'Optioneel',
  'Email address': 'E-mailadres',
  'you@municipality.be': 'jij@gemeente.be',
  'We’ll send one completion notice for this task.':
    'Je ontvangt één e-mail zodra deze taak klaar is.',
  'Email notifications are available once a mail service is connected.':
    'E-mailmeldingen zijn beschikbaar zodra een maildienst is gekoppeld.',
  'Your original data stays intact':
    'Je oorspronkelijke gegevens blijven behouden',
  'Reading your file': 'Bestand wordt ingelezen',
  'Organise file': 'Bestand ordenen',
  'All your columns. All your records.': 'Al je kolommen. Al je records.',
  'Nothing lost along the way.': 'Er gaat niets verloren.',
  'Your file is in good hands': 'Je bestand is in goede handen',
  'Putting the details in order.': 'We brengen alles op orde.',
  'We’re checking the data in your file. Any uncertainty will be marked for review.':
    'We controleren de gegevens in je bestand. Bij twijfel markeren we ze om na te kijken.',
  'Opening your task…': 'Je taak wordt geopend…',
  'Reading file': 'Bestand wordt ingelezen',
  'Comparing Google Maps candidates': 'Suggesties uit Google Maps vergelijken',
  'Checking records': 'Records controleren',
  'Records processed': 'Verwerkte records',
  of: 'van',
  'records processed': 'records verwerkt',
  'Read & organise': 'Inlezen en ordenen',
  'Identify columns and preserve source data':
    'Kolommen herkennen en brongegevens behouden',
  Complete: 'Voltooid',
  'Check data quality': 'Datakwaliteit controleren',
  'Identifiers, duplicate records and missing fields':
    'Nummers, dubbele records en ontbrekende velden',
  'In progress': 'Bezig',
  'Compare business information': 'Bedrijfsinformatie vergelijken',
  'Google Maps candidates are suggestions to review':
    'Suggesties uit Google Maps moet je zelf nakijken',
  'Google Maps comparison was not requested':
    'Vergelijking met Google Maps is niet aangevraagd',
  Queued: 'In de wachtrij',
  Skipped: 'Overgeslagen',
  'No need to keep watching.': 'Je hoeft niet te blijven wachten.',
  'Leave your email and we’ll let you know when it’s ready.':
    'Laat je e-mailadres achter. We laten je weten wanneer het klaar is.',
  'This task keeps running if you close the tab. Bookmark this page to return.':
    'Deze taak loopt door als je het tabblad sluit. Bewaar deze pagina als bladwijzer om terug te keren.',
  'Notification email': 'E-mailadres voor meldingen',
  'Notify me': 'Houd me op de hoogte',
  'File organised': 'Bestand geordend',
  'Ready for a closer look.': 'Klaar om na te kijken.',
  'records, original data preserved':
    'records, oorspronkelijke gegevens behouden',
  'Upload another file': 'Nog een bestand uploaden',
  'Your file is organised.': 'Je bestand is geordend.',
  'No unresolved checks remain.': 'Er zijn geen openstaande controles meer.',
  'Google Maps candidates require a human check.':
    'Suggesties uit Google Maps moeten door een persoon worden nagekeken.',
  'Source data checked. Google Maps comparison was not run.':
    'Brongegevens gecontroleerd. Geen vergelijking met Google Maps uitgevoerd.',
  'Download JSON': 'JSON downloaden',
  'Other download formats': 'Andere downloadformaten',
  'Other formats': 'Andere formaten',
  'Download CSV': 'CSV downloaden',
  'Download GeoJSON': 'GeoJSON downloaden',
  'Business records': 'Bedrijfsgegevens',
  'Filter records': 'Records filteren',
  'All records': 'Alle records',
  'Needs review': 'Na te kijken',
  Reviewed: 'Nagekeken',
  'Missing contact info': 'Contactgegevens ontbreken',
  'Search businesses': 'Bedrijven zoeken',
  'Search this file': 'Dit bestand doorzoeken',
  'Find businesses': 'Vind bedrijven',
  'Industry, name, location or keyword…': 'Sector, naam, locatie of trefwoord…',
  'Clear search': 'Zoekopdracht wissen',
  'Filter by industry': 'Filteren op sector',
  'All industries': 'Alle sectoren',
  'Filter by category': 'Filteren op categorie',
  'All categories': 'Alle categorieën',
  Category: 'Categorie',
  'No category fields in this file. Add a category column to filter by category.':
    'Dit bestand bevat geen categorievelden. Voeg een categoriekolom toe om op categorie te filteren.',
  'Search across this file, including contact details, notes and original fields. Combine keywords to narrow results.':
    'Doorzoek dit bestand, inclusief contactgegevens, notities en oorspronkelijke velden. Combineer trefwoorden om gerichter te zoeken.',
  'No industry fields in this file. Other keywords are still searchable.':
    'Dit bestand bevat geen sectorvelden. Je kunt wel op andere trefwoorden zoeken.',
  '{count} of {total} records match': '{count} van {total} records gevonden',
  'Reset search and filters': 'Zoekopdracht en filters wissen',

  'Find a name, number or address…': 'Zoek een naam, nummer of adres…',
  'Business / identifier': 'Bedrijf / nummer',
  Address: 'Adres',
  'Source status': 'Status in bron',
  Review: 'Nakijken',
  Actions: 'Acties',
  'Unnamed business': 'Bedrijf zonder naam',
  'No identifier': 'Geen nummer',
  Establishment: 'Vestiging',
  Enterprise: 'Onderneming',
  'No address provided': 'Geen adres opgegeven',
  'Not provided': 'Niet opgegeven',
  'Checks passed': 'Controles geslaagd',
  'Google Maps candidate': 'Suggestie uit Google Maps',
  'Google Maps unavailable': 'Google Maps niet beschikbaar',
  'Source checks only': 'Alleen broncontroles',
  'No records match this view': 'Geen records gevonden',
  'Try another search or return to all records.':
    'Probeer een andere zoekopdracht of bekijk alle records.',
  'Show all records': 'Alle records tonen',
  '0 records': '0 records',
  Previous: 'Vorige',
  Next: 'Volgende',
  'Corrections are saved separately. Every download includes original source fields.':
    'Wijzigingen worden apart opgeslagen. Elke download bevat de oorspronkelijke bronvelden.',
  'No completion email requested': 'Geen e-mailmelding aangevraagd',
  'Email me a completion notice': 'Stuur me een e-mail zodra het klaar is',
  'Send notice': 'Melding versturen',
  'Business data, in order.': 'Bedrijfsgegevens op orde.',
  'Built for local public services': 'Gemaakt voor lokale overheidsdiensten',
  Record: 'Record',
  'Review business details': 'Bedrijfsgegevens nakijken',
  'Close record': 'Record sluiten',
  'Parent enterprise:': 'Bovenliggende onderneming:',
  'Check before using this record': 'Controleer dit record vóór gebruik',
  'Business name': 'Bedrijfsnaam',
  Phone: 'Telefoon',
  Email: 'E-mail',
  Website: 'Website',
  'Registered status in source': 'Geregistreerde status in bron',
  'This is the uploaded value, not a live KBO verification.':
    'Dit is de geüploade waarde, geen actuele controle bij de KBO.',
  'Google Maps · Unconfirmed candidate': 'Google Maps · Onbevestigde suggestie',
  'Business status': 'Bedrijfsstatus',
  'Confirm this is the same establishment before making any corrections. Google closure status is not legal registration status.':
    'Controleer of dit dezelfde vestiging is voordat je iets wijzigt. Een sluitingsstatus in Google Maps is geen wettelijke registratiestatus.',
  'Open in Google Maps': 'Openen in Google Maps',
  '. No external information was confirmed.':
    '. Er is geen externe informatie bevestigd.',
  'Review notes': 'Notities bij de controle',
  'Record your source or explain a correction…':
    'Vermeld je bron of licht een wijziging toe…',
  'I have reviewed this record': 'Ik heb dit record nagekeken',
  'View original source fields': 'Oorspronkelijke bronvelden bekijken',
  Cancel: 'Annuleren',
  'Saving…': 'Bezig met opslaan…',
  'Save changes': 'Wijzigingen opslaan',
  'Something went wrong. Please try again.':
    'Er ging iets mis. Probeer het opnieuw.',
  'The processing service is unavailable. Please start the app with npm run dev.':
    'De verwerkingsdienst is niet beschikbaar. Start de app met npm run dev.',
  'The processing service is unavailable. Please try again shortly.':
    'De verwerkingsdienst is niet beschikbaar. Probeer het zo opnieuw.',
  'Choose a CSV, JSON or GeoJSON file.':
    'Kies een CSV-, JSON- of GeoJSON-bestand.',
  'This file is too large. Choose a file smaller than 10 MB.':
    'Dit bestand is te groot. Kies een bestand kleiner dan 10 MB.',
  'This file is empty. Choose a file that contains records.':
    'Dit bestand is leeg. Kies een bestand met records.',
  'Processing failed. Please upload the file again.':
    'De verwerking is mislukt. Upload het bestand opnieuw.',
  'Saved. We’ll email you when your file is ready.':
    'Opgeslagen. Je ontvangt een e-mail zodra je bestand klaar is.',
  'Upload a file smaller than 10 MB.': 'Upload een bestand kleiner dan 10 MB.',
  'A valid email and a configured notification service are required.':
    'Een geldig e-mailadres en een ingestelde meldingsdienst zijn vereist.',
  'Google Maps is not connected.': 'Google Maps is niet verbonden.',
  'Two files are already processing. Please try again shortly.':
    'Er worden al twee bestanden verwerkt. Probeer het zo opnieuw.',
  'Could not create a job.': 'De taak kon niet worden aangemaakt.',
  'Could not save the upload.': 'De upload kon niet worden opgeslagen.',
  'This task was not found. Upload your file to start again.':
    'Deze taak is niet gevonden. Upload je bestand om opnieuw te beginnen.',
  'Invalid update.': 'Ongeldige wijziging.',
  'Send either a record edit or an email update.':
    'Verstuur een recordwijziging of een e-mailwijziging.',
  'Task not found.': 'Taak niet gevonden.',
  'Enter a valid email. Notifications must be configured.':
    'Voer een geldig e-mailadres in. Meldingen moeten zijn ingesteld.',
  'A completion email has already been queued.':
    'Er staat al een e-mailmelding in de wachtrij.',
  'Wait until processing finishes.': 'Wacht tot de verwerking is voltooid.',
  'Record not found.': 'Record niet gevonden.',
  'Your changes could not be saved. Please try again.':
    'Je wijzigingen konden niet worden opgeslagen. Probeer het opnieuw.',
  'Could not read this CSV': 'Dit CSV-bestand kon niet worden gelezen',
  'The CSV needs a header row':
    'Het CSV-bestand moet een rij met kolomnamen bevatten',
  'CSV column names must be non-empty and unique':
    'CSV-kolomnamen mogen niet leeg zijn en moeten uniek zijn',
  'Upload up to 10,000 records per file':
    'Upload maximaal 10.000 records per bestand',
  'This file is not valid JSON': 'Dit bestand bevat geen geldige JSON',
  'Unexpected content after the JSON document':
    'Onverwachte inhoud na het JSON-document',
  'GeoJSON must contain a features array':
    'GeoJSON moet een features-array bevatten',
  'GeoJSON contains an invalid feature': 'GeoJSON bevat een ongeldige feature',
  'Each feature needs properties': 'Elke feature moet properties bevatten',
  'JSON must be an array, a records object, or a GeoJSON FeatureCollection':
    'JSON moet een array, een records-object of een GeoJSON FeatureCollection zijn',
  'JSON must contain records': 'JSON moet records bevatten',
  'Each record must be a JSON object': 'Elk record moet een JSON-object zijn',
  'Choose a .csv, .json or .geojson file':
    'Kies een .csv-, .json- of .geojson-bestand',
  'The file has no records': 'Het bestand bevat geen records',
  'No business columns found. Include name or Maatschappelijke_naam, and number or Ondernemingsnr. Download the example for a template.':
    'Geen bedrijfskolommen gevonden. Voeg name of Maatschappelijke_naam en number of Ondernemingsnr toe. Download het voorbeeld als sjabloon.',
  'Check identifier': 'Controleer het nummer',
  'Missing business name': 'Bedrijfsnaam ontbreekt',
  'Missing address': 'Adres ontbreekt',
  'Check email format': 'Controleer het e-mailadres',
  'No coordinates': 'Geen coördinaten',
  'Check registered status': 'Controleer de geregistreerde status',
  'Duplicate identifier': 'Dubbel nummer',
  'Missing name or address; lookup skipped':
    'Naam of adres ontbreekt; opzoeken overgeslagen',
  'Google Maps could not be reached': 'Google Maps kon niet worden bereikt',
  'Google Maps returned an unreadable response':
    'Google Maps gaf een onleesbaar antwoord',
  'No candidate found': 'Geen suggestie gevonden',
  'Not found': 'Niet gevonden',
  'Invalid request origin': 'Ongeldige herkomst van het verzoek',
  'Failed to fetch': 'Verbinding mislukt. Probeer het opnieuw.',
  'Load failed': 'Laden mislukt. Probeer het opnieuw.',
  'not requested': 'niet aangevraagd',
  pending: 'in afwachting',
  sending: 'wordt verstuurd',
  sent: 'verstuurd',
  failed: 'mislukt',
  establishment: 'vestiging',
  enterprise: 'onderneming',
  Language: 'Taal',
  '{count} records need a closer look.':
    '{count} records moeten worden nagekeken.',
  '{start}–{end} of {count} records': '{start}–{end} van {count} records',
  '{count} records': '{count} records',
  '{done} of {total} records processed': '{done} van {total} records verwerkt',
  'A completion notice is requested for {email}':
    'Een e-mailmelding is aangevraagd voor {email}',
  'Email notification: {status}': 'E-mailmelding: {status}',
  'Review {name}': '{name} nakijken',
  'CSV row {row} is malformed: check its column count and quotes':
    'CSV-rij {row} is ongeldig: controleer het aantal kolommen en de aanhalingstekens',
  'Google Maps returned {status}; no match confirmed':
    'Google Maps gaf code {status}; geen overeenkomst bevestigd'
};

// Translate interface messages only; uploaded fields and exported records stay intact.
export function translate(
  locale: Locale,
  message: string,
  values: Record<string, string | number> = {}
): string {
  let translated = locale === 'nl' ? (nl[message] ?? message) : message;
  return translated.replace(/\{(\w+)\}/g, (token, key) =>
    String(values[key] ?? token)
  );
}
export function translateMessage(locale: Locale, message: string): string {
  const csv =
    /^CSV row (\d+) is malformed: check its column count and quotes$/.exec(
      message
    );
  if (csv)
    return translate(
      locale,
      'CSV row {row} is malformed: check its column count and quotes',
      { row: csv[1] }
    );
  const maps = /^Google Maps returned (\d+); no match confirmed$/.exec(message);
  if (maps)
    return translate(
      locale,
      'Google Maps returned {status}; no match confirmed',
      { status: maps[1] }
    );
  return translate(locale, message);
}
