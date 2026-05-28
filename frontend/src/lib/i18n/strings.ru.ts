// Russian translation kept as backup for a future runtime locale-switching PR. Not imported anywhere today.
export const streamErrorReasons: Record<string, string> = {
  timeout: 'Превышено время ожидания (3 сек)',
  negotiation_failed: 'Ошибка согласования соединения',
  ice_failed: 'Не удалось установить медиа-канал',
  network: 'Сетевая ошибка',
  unknown: 'Неизвестная ошибка'
}

export const ui = {
  // grid mode toggle
  hd: 'HD',
  // stream errors
  retry: 'Повторить',
  streamErrorHeading: 'Не удалось подключиться к камере',
  connecting: 'Подключение...',
  buffering: 'Буферизация...',
  // navigation
  cameras: 'Камеры',
  events: 'События',
  settings: 'Настройки',
  // shared
  comingSoon: 'Скоро',
  online: 'В сети',
  offline: 'Офлайн',
  live: 'LIVE',
  // settings
  showTimestamp: 'Время на тайлах и видео',
  // events
  eventsTitle: 'События',
  eventsEmpty: 'Нет событий',
  eventsLoading: 'Загрузка...',
  eventsLoadError: 'Не удалось загрузить события',
  filterAll: 'Все',
  recentEvents: 'Недавние события',
  noRecentEvents: 'Пока ничего',
  openInFrigate: 'Открыть в Frigate',
  downloadVideo: 'Скачать видео',
  close: 'Закрыть',
  duration: 'Длительность',
  score: 'Уверенность',
  // settings > cameras section (E5)
  camerasSection: 'Камеры',
  refreshCameras: 'Обновить из Frigate',
  refreshingCameras: 'Обновление…',
  refreshNoChanges: 'Изменений нет.',
  refreshFrigateDown: 'Frigate недоступен.',
  refreshGenericError: 'Ошибка обновления.',
  refreshDiffAdded: 'Добавлено',
  refreshDiffRemoved: 'Удалено',
  // settings > IA + appearance + per-camera editor (E6 sprint B)
  sectionAppearance: 'Внешний вид',
  sectionCameras: 'Камеры',
  sectionGroups: 'Группы',
  gridModeLabel: 'Режим сетки',
  desktopColumnsLabel: 'Колонки (десктоп)',
  mobileColumnsLabel: 'Колонки (мобильный)',
  nameStyleLabel: 'Имя камеры',
  accentLabel: 'Акцент',
  nameLabel: 'Имя',
  streamMain: 'Основной поток',
  streamSub: 'Доп. поток',
  streamDefault: 'По умолчанию',
  streamsHint: 'Источники видео в go2rtc. Меняйте, только если знаете, что делаете.',
  streamsLoading: 'Загрузка потоков…',
  streamsLoadError: 'Не удалось загрузить список потоков go2rtc.',
  streamOverrideSaveError: 'Не удалось сохранить.',
  streamOverrideSaving: 'Сохранение…',
  // settings > shared editor labels
  groupNameLabel: 'Название',
  groupNoCameras: 'Нет камер',
  groupCameras: 'Камеры:',
  cancel: 'Отмена',
  save: 'Сохранить',
  edit: 'Редактировать',
  delete: 'Удалить',
  createGroup: '+ Создать группу',
  newGroupPlaceholder: 'Название группы',
  create: 'Создать',
  confirmDeleteGroup: 'Удалить группу «{name}»? Камеры останутся, но не будут в группе.',
  saveErrorGeneric: 'Ошибка сохранения',
  createErrorGeneric: 'Ошибка создания',
  // shared labels
  yes: 'Да',
  no: 'Нет',
  cameraNameAria: 'Имя камеры',
  refreshHint:
    'Перечитать список камер из Frigate. Используйте после добавления или удаления камеры в Frigate.',
  // moved out of components in L3
  errorTitle: 'Что-то пошло не так',
  errorSubtitle: 'Перезагрузите страницу, чтобы продолжить.',
  errorReload: 'Перезагрузить',
  menuLabel: 'Меню',
  telemetryLabel: 'Телеметрия',
  otherCamerasLabel: 'Другие камеры',
  primaryNavLabel: 'Основная навигация',
  groupFilterLabel: 'Фильтр групп',
  cameraGroupTitle: 'Группа камер',
  camerasCountWord: 'камер',
  backLabel: 'Назад',
  streamLabel: 'Поток',
  qualityLabel: 'Качество',
  viewAll: 'посм. все →',
  secondsShort: 'с',
  noSignal: 'нет сигнала',
  statusUnknown: 'Статус неизвестен',
  installAppTitle: 'Установить приложение',
  install: 'Установить',
  installIOSHintTap: 'Нажмите',
  installIOSHintShare: '⬆ Поделиться',
  installIOSHintHome: 'На экран «Домой»',
  appUpdatedToast: 'Приложение обновлено',
  groupAria: 'Группа',
  cameraAria: 'Камера',
  kindAria: 'Тип',
  nameStyleBelow: 'Снизу',
  nameStyleOverlay: 'Поверх',
  accentCyan: 'Синий',
  accentSage: 'Зелёный',
  accentAmber: 'Жёлтый',
  accentViolet: 'Фиолетовый'
}

// eventKindLabels covers the four EventKind values surfaced by the BFF.
// 'motion' from the design mock is gone — Frigate `motion` collapses into
// 'other' server-side (see backend/internal/events.KindFor).
export const eventKindLabels: Record<string, string> = {
  person: 'Человек',
  vehicle: 'Машина',
  animal: 'Животное',
  other: 'Другое'
}
