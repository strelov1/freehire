import { defineMessages } from '$lib/i18n/t';

// English builds the Stepper's full label by appending " margin" to the bare side
// name ("Left" -> "Left margin"); Russian bakes the noun into the adjective instead
// ("Левое поле" stands on its own), so each side carries two forms rather than one
// suffix rule that only works in English — see design.md's "word order and
// inflection are a property of the language" principle.
export const messages = defineMessages(
  {
    axes: {
      sides: 'Side margins',
      ends: 'Top & bottom',
    },
    sides: {
      left: { row: 'Left', stepper: 'Left margin' },
      right: { row: 'Right', stepper: 'Right margin' },
      top: { row: 'Top', stepper: 'Top margin' },
      bottom: { row: 'Bottom', stepper: 'Bottom margin' },
    },
    sidesDiffer: 'Sides differ',
    linkMargins: 'Link the margins',
    setEachSide: 'Set each side separately',
  },
  {
    ru: {
      axes: {
        sides: 'Боковые поля',
        ends: 'Отступы сверху и снизу',
      },
      sides: {
        left: { row: 'Левое поле', stepper: 'Левое поле' },
        right: { row: 'Правое поле', stepper: 'Правое поле' },
        top: { row: 'Верхнее поле', stepper: 'Верхнее поле' },
        bottom: { row: 'Нижнее поле', stepper: 'Нижнее поле' },
      },
      sidesDiffer: 'Стороны разные',
      linkMargins: 'Связать поля',
      setEachSide: 'Настроить каждую сторону отдельно',
    },
  },
);
