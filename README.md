# go-bpm-ai-orchestrator

Тестовый проект: два LLM-агента работают в процессе BPMN, который исполняет go-bpm на PostgreSQL. Исполнитель готовит решение, отдельная модель проверяет результат, замечания возвращаются исполнителю. Процесс ограничивает число итераций и сохраняет историю.

Самостоятельная копия [go-bpm](https://github.com/nicolas-vp/go-bpm), исходная версия `17a32308eed8d4d0c17c68d190a23eb5719ee685`. Движок, генератор, миграции, HTTP API и консоль находятся здесь. Все исходные демопримеры удалены; зарегистрирован только процесс `AiOrchestrator`. Соседний checkout go-bpm не нужен.

![Процесс в консоли](docs/images/02-history.png)

## Запуск

Нужны Docker Compose и доступная Ollama с установленными моделями:

```sh
cp .env.example .env
# Заполните POSTGRES_PASSWORD: используйте буквы и цифры для корректного DSN.
# При необходимости измените OLLAMA_URL и имена моделей.
docker compose up -d --build
```

Консоль: http://localhost:8092/ui/. База и том `go-bpm-ai-orchestrator_aidata` собственные. Порт опубликован только на localhost. API и консоль не имеют авторизации.

Модели по умолчанию:

| Роль | Модель | Контекст в Modelfile стенда |
| --- | --- | --- |
| Исполнитель | `qwen3-coder:30b-32k` | 32768 |
| Рецензент | `qwen2.5-coder:32b-8k` | 8192 |

Адаптер **не переопределяет** temperature, num_ctx, num_predict и другие параметры модели. Вызовы последовательные, `keep_alive: 0` освобождает память после ответа: две большие модели не нужно держать на GPU одновременно. У каждой роли собственный system prompt и отдельный запрос без общей истории чата. Если нужна одна модель в двух ролях, укажите одинаковые WRITER_MODEL и REVIEWER_MODEL.

Для локального запуска нужны Go 1.26+ и отдельная PostgreSQL:

```sh
export GO_BPM_DSN='postgres://user:password@localhost:5432/ai_orchestrator?sslmode=disable'
make build
bin/orchestrator -addr 127.0.0.1:8092
```

## Пройти процесс

Через API достаточно передать `goal` и необязательный `maxRounds` (по умолчанию 3, диапазон 1–5):

```sh
curl -fsS -X POST http://localhost:8092/api/v1/processes/AiOrchestrator/instances \
  -H 'Content-Type: application/json' \
  -d '{"businessKey":"unique-sorted-1","variables":{"goal":"Write a Go function UniqueSorted(values []int) []int. Return unique integers sorted ascending without mutating the input. For nil or empty input return a non-nil empty slice. Include table-driven tests for these requirements.","maxRounds":3}}'
```

В ответе — `id`. Откройте `http://localhost:8092/ui/history/?instanceId=<id>`. Нажмите на AI-задачу, чтобы увидеть черновик, замечания и переменные каждого прохода. Запустить процесс можно также во вкладке «Запуск», выбрав `AiOrchestrator` и указав переменную `goal`.

```mermaid
flowchart LR
  Start --> Prepare[Новая итерация]
  Prepare --> Write[Исполнитель Qwen3]
  Write --> Review[Рецензент Qwen2.5]
  Review --> Verdict{Принято?}
  Verdict -->|да| Accepted[outcome: accepted]
  Verdict -->|нет| Limit{Есть попытки?}
  Limit -->|да| Prepare
  Limit -->|нет| Human[outcome: needs_human]
```

`round` считает пары исполнитель/рецензент. `draft` содержит последний ответ исполнителя. `review` — JSON с обязательными `approved` (boolean) и непустым `feedback`. Некорректный JSON, пустой или обрезанный ответ, HTTP-ошибка вызывают технический повтор задачи (2 попытки), затем статус экземпляра `failed`. Это отдельный механизм от бизнес-цикла доработок.

Оба бизнес-исхода имеют статус экземпляра `completed`; различайте их по переменной `outcome`. `needs_human` сохраняет результат для дальнейшего разбора, но не создаёт пользовательскую задачу и не отправляет уведомления.

Блок упавших задач на странице истории позволяет повторить задачу после устранения причины. Повтор продолжает сохранённый экземпляр. Вызов LLM при сбое после ответа может повториться: внешняя генерация не имеет гарантии exactly-once.

## Проверки

```sh
make generate
make build
make test
make vet
# Отдельная тестовая база: таблицы в ней очищаются!
GO_BPM_TEST_DSN='postgres://user:password@localhost:5432/ai_orchestrator_test?sslmode=disable' make test
```

Без GO_BPM_TEST_DSN интеграционные тесты пропускаются. Тесты адаптера проверяют выбор модели, наследование параметров и отбрасывание некорректных ответов. Интеграционные сценарии используют настоящий PostgreSQL и сгенерированный BPMN с детерминированными ответами: принятие с первого раза, доработка с передачей замечаний и лимит итераций.

## Материалы эксперимента

- [Архитектура, статья и границы примера](docs/design.md).
- [Журнал, проверка стенда и скриншоты](docs/experiment/README.md).
- [Параметры моделей на стенде](docs/experiment/models.json).
- [BPMN — источник процесса](process/orchestrator.bpmn).

Это демонстрация оркестрации генерации и ревью текста/кода. Модели не получают shell, файловую систему или GitHub. Проект не реализует автономную разработку в нескольких репозиториях, запуск тестов моделью, merge и деплой. Одобрение второй LLM не доказывает корректность кода: в сохранённом реальном прогоне рецензент принял ответ, который затем не прошёл компиляцию. Подробности и исходный ответ — в журнале эксперимента.

Apache-2.0, см. [LICENSE](LICENSE) и [NOTICE](NOTICE).
