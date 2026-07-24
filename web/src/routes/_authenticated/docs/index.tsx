/*
Copyright (C) 2026 xju-api contributors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { createFileRoute } from '@tanstack/react-router'

import { TutorialDocumentation } from '@/features/tutorial-docs'

export const Route = createFileRoute('/_authenticated/docs/')({
  component: TutorialDocumentation,
})
