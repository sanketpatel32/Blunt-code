import { describe, expect, it } from 'vitest';
import { isTextEntryTarget, parseShortcut } from './shortcuts';

describe('parseShortcut', () => {
  it('normalizes plain letters to lowercase shortcut keys', () => {
    expect(parseShortcut({ key: 'g' })).toBe('g');
    expect(parseShortcut({ key: 'h' })).toBe('h');
    expect(parseShortcut({ key: 'w' })).toBe('w');
    expect(parseShortcut({ key: 't' })).toBe('t');
    expect(parseShortcut({ key: 's' })).toBe('s');
    expect(parseShortcut({ key: 'n' })).toBe('n');
    expect(parseShortcut({ key: 'H' })).toBe('h'); // CapsLock or Shift still normalizes
  });

  it('recognizes the punctuation shortcuts, including Shift+/', () => {
    expect(parseShortcut({ key: '/' })).toBe('/');
    expect(parseShortcut({ key: '?', shiftKey: true })).toBe('?');
    expect(parseShortcut({ key: '?' })).toBe('?');
  });

  it('returns null for keys that are not shortcuts', () => {
    expect(parseShortcut({ key: 'a' })).toBeNull();
    expect(parseShortcut({ key: 'Escape' })).toBeNull();
    expect(parseShortcut({ key: 'Enter' })).toBeNull();
    expect(parseShortcut({ key: 'Tab' })).toBeNull();
    expect(parseShortcut({ key: '' })).toBeNull();
  });

  it('returns null when a browser/screen-reader modifier is held', () => {
    expect(parseShortcut({ key: 'g', ctrlKey: true })).toBeNull();
    expect(parseShortcut({ key: 'g', metaKey: true })).toBeNull();
    expect(parseShortcut({ key: 'g', altKey: true })).toBeNull();
  });
});

describe('isTextEntryTarget', () => {
  it('is true for inputs, textareas and selects, including when nested', () => {
    const input = document.createElement('input');
    const label = document.createElement('label');
    label.append(input);
    expect(isTextEntryTarget(input)).toBe(true);
    expect(isTextEntryTarget(label.querySelector('input'))).toBe(true);
    expect(isTextEntryTarget(document.createElement('textarea'))).toBe(true);
    expect(isTextEntryTarget(document.createElement('select'))).toBe(true);
  });

  it('is true inside contenteditable regions and false when editing is disabled', () => {
    const editor = document.createElement('div');
    editor.setAttribute('contenteditable', 'true');
    const child = document.createElement('span');
    editor.append(child);
    expect(isTextEntryTarget(editor)).toBe(true);
    expect(isTextEntryTarget(child)).toBe(true);
    const frozen = document.createElement('div');
    frozen.setAttribute('contenteditable', 'false');
    expect(isTextEntryTarget(frozen)).toBe(false);
  });

  it('is false for plain elements and non-element targets', () => {
    expect(isTextEntryTarget(document.createElement('button'))).toBe(false);
    expect(isTextEntryTarget(document.createElement('div'))).toBe(false);
    expect(isTextEntryTarget(null)).toBe(false);
    expect(isTextEntryTarget(window)).toBe(false);
  });
});
