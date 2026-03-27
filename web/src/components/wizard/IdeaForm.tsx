import { useState, useRef, useCallback } from 'react'

const LANGUAGES = [
  { value: 'en-US', label: '\uD83C\uDDFA\uD83C\uDDF8 English' },
  { value: 'pl-PL', label: '\uD83C\uDDF5\uD83C\uDDF1 Polski' },
  { value: 'de-DE', label: '\uD83C\uDDE9\uD83C\uDDEA Deutsch' },
  { value: 'es-ES', label: '\uD83C\uDDEA\uD83C\uDDF8 Espa\u00F1ol' },
  { value: 'fr-FR', label: '\uD83C\uDDEB\uD83C\uDDF7 Fran\u00E7ais' },
  { value: 'pt-PT', label: '\uD83C\uDDF5\uD83C\uDDF9 Portugu\u00EAs' },
  { value: 'it-IT', label: '\uD83C\uDDEE\uD83C\uDDF9 Italiano' },
  { value: 'nl-NL', label: '\uD83C\uDDF3\uD83C\uDDF1 Nederlands' },
  { value: 'ru-RU', label: '\uD83C\uDDF7\uD83C\uDDFA \u0420\u0443\u0441\u0441\u043A\u0438\u0439' },
  { value: 'zh-CN', label: '\uD83C\uDDE8\uD83C\uDDF3 \u4E2D\u6587' },
  { value: 'ja-JP', label: '\uD83C\uDDEF\uD83C\uDDF5 \u65E5\u672C\u8A9E' },
  { value: 'ko-KR', label: '\uD83C\uDDF0\uD83C\uDDF7 \uD55C\uAD6D\uC5B4' },
] as const

interface IdeaFormProps {
  onSubmit: (data: {
    type: string
    idea: string
    language: string
    addToSprint: boolean
  }) => void
  isLoading: boolean
}

export function IdeaForm({ onSubmit, isLoading }: IdeaFormProps) {
  const [type, setType] = useState<string | null>(null)
  const [idea, setIdea] = useState('')
  const [language, setLanguage] = useState('en-US')
  const [addToSprint, setAddToSprint] = useState(true)
  
  // Audio recording state
  const [isRecording, setIsRecording] = useState(false)
  const [recordingTime, setRecordingTime] = useState(0)
  const mediaRecorderRef = useRef<MediaRecorder | null>(null)
  const audioChunksRef = useRef<Blob[]>([])
  const recordingTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // Check if browser supports speech recognition
  const supportsSpeechRecognition = 'webkitSpeechRecognition' in window || 'SpeechRecognition' in window

  // Type selection screen
  if (!type) {
    return (
      <div>
        <h2 className="text-xl font-bold text-white mb-6 text-center">
          Select Issue Type
        </h2>
        <div className="grid grid-cols-2 gap-4 max-w-md mx-auto">
          <button
            type="button"
            onClick={() => setType('feature')}
            className="border-2 border-gray-700 rounded-lg p-6 text-center hover:border-blue-500 hover:-translate-y-0.5 transition-all bg-gray-900"
          >
            <div className="text-3xl mb-2">{'\u2728'}</div>
            <div className="font-semibold text-gray-200">Feature</div>
            <div className="text-xs text-gray-500 mt-1">
              New functionality or enhancement
            </div>
          </button>
          <button
            type="button"
            onClick={() => setType('bug')}
            className="border-2 border-gray-700 rounded-lg p-6 text-center hover:border-red-500 hover:-translate-y-0.5 transition-all bg-gray-900"
          >
            <div className="text-3xl mb-2">{'\uD83D\uDC1B'}</div>
            <div className="font-semibold text-gray-200">Bug</div>
            <div className="text-xs text-gray-500 mt-1">
              Something is not working correctly
            </div>
          </button>
        </div>
      </div>
    )
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!idea.trim()) return
    onSubmit({ type, idea: idea.trim(), language, addToSprint })
  }

  // Start audio recording
  const startRecording = useCallback(async () => {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      const mediaRecorder = new MediaRecorder(stream)
      mediaRecorderRef.current = mediaRecorder
      audioChunksRef.current = []

      mediaRecorder.ondataavailable = (event) => {
        if (event.data.size > 0) {
          audioChunksRef.current.push(event.data)
        }
      }

      mediaRecorder.onstop = () => {
        // Audio recording stopped - in production, send to speech-to-text API
        // For now, just indicate that recording is complete
        setIdea((prev) => prev + (prev ? '\n\n' : '') + '[Audio recorded - transcription would appear here]')
        
        // Stop all tracks
        stream.getTracks().forEach(track => track.stop())
      }

      mediaRecorder.start()
      setIsRecording(true)
      setRecordingTime(0)
      
      // Start timer
      recordingTimerRef.current = setInterval(() => {
        setRecordingTime(prev => prev + 1)
      }, 1000)
    } catch (err) {
      console.error('Error accessing microphone:', err)
      alert('Could not access microphone. Please check permissions.')
    }
  }, [])

  // Stop audio recording
  const stopRecording = useCallback(() => {
    if (mediaRecorderRef.current && isRecording) {
      mediaRecorderRef.current.stop()
      setIsRecording(false)
      
      if (recordingTimerRef.current) {
        clearInterval(recordingTimerRef.current)
        recordingTimerRef.current = null
      }
    }
  }, [isRecording])

  // Toggle recording
  const toggleRecording = useCallback(() => {
    if (isRecording) {
      stopRecording()
    } else {
      startRecording()
    }
  }, [isRecording, startRecording, stopRecording])

  // Format recording time
  const formatTime = (seconds: number) => {
    const mins = Math.floor(seconds / 60)
    const secs = seconds % 60
    return `${mins}:${secs.toString().padStart(2, '0')}`
  }

  return (
    <div>
      <h2 className="text-xl font-bold text-white mb-6">
        Create New {type === 'bug' ? 'Bug Report' : 'Feature'}
      </h2>

      <form onSubmit={handleSubmit}>
        <div className="mb-4">
          <label
            htmlFor="wizard-idea"
            className="block text-sm text-gray-400 mb-2"
          >
            Describe your {type === 'bug' ? 'bug' : 'feature idea'}:
          </label>
          <div className="relative">
            <textarea
              id="wizard-idea"
              value={idea}
              onChange={(e) => setIdea(e.target.value)}
              rows={6}
              required
              className="w-full px-3 py-2 bg-gray-900 border border-gray-700 rounded-lg text-gray-200 text-sm resize-y focus:outline-none focus:border-blue-500 transition-colors font-sans pr-12"
              placeholder={
                type === 'bug'
                  ? 'Describe the bug, steps to reproduce, and expected behavior...'
                  : 'Describe the feature, who it\'s for, and what problem it solves...'
              }
            />
            
            {/* Microphone button */}
            <button
              type="button"
              onClick={toggleRecording}
              disabled={!supportsSpeechRecognition}
              className={`absolute bottom-3 right-3 p-2 rounded-full transition-all ${
                isRecording
                  ? 'bg-red-500/20 text-red-400 animate-pulse'
                  : 'bg-gray-800 text-gray-400 hover:bg-gray-700 hover:text-gray-200'
              } ${!supportsSpeechRecognition ? 'opacity-50 cursor-not-allowed' : ''}`}
              title={
                !supportsSpeechRecognition
                  ? 'Speech recognition not supported in this browser'
                  : isRecording
                  ? 'Stop recording'
                  : 'Start voice recording'
              }
            >
              {isRecording ? (
                <svg viewBox="0 0 24 24" fill="currentColor" className="w-5 h-5">
                  <rect x="6" y="6" width="12" height="12" rx="2" />
                </svg>
              ) : (
                <svg viewBox="0 0 24 24" fill="currentColor" className="w-5 h-5">
                  <path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3z" />
                  <path d="M17 11c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z" />
                </svg>
              )}
            </button>
          </div>
          
          {/* Recording indicator */}
          {isRecording && (
            <div className="flex items-center gap-2 mt-2 text-xs text-red-400">
              <span className="w-2 h-2 bg-red-500 rounded-full animate-pulse" />
              <span>Recording... {formatTime(recordingTime)}</span>
              <span className="text-gray-500">(Click microphone to stop)</span>
            </div>
          )}
        </div>

        <div className="mb-4">
          <label
            htmlFor="wizard-language"
            className="block text-sm text-gray-400 mb-2"
          >
            Language:
          </label>
          <select
            id="wizard-language"
            value={language}
            onChange={(e) => setLanguage(e.target.value)}
            className="px-3 py-2 bg-gray-900 border border-gray-700 rounded-lg text-gray-200 text-sm focus:outline-none focus:border-blue-500 transition-colors cursor-pointer"
          >
            {LANGUAGES.map((l) => (
              <option key={l.value} value={l.value}>
                {l.label}
              </option>
            ))}
          </select>
        </div>

        <div className="mb-6">
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={addToSprint}
              onChange={(e) => setAddToSprint(e.target.checked)}
              className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-blue-600 focus:ring-blue-500 cursor-pointer"
            />
            <span className="text-sm text-gray-300">
              Add to current sprint
            </span>
          </label>
        </div>

        <div className="flex justify-between items-center">
          <button
            type="button"
            onClick={() => setType(null)}
            className="px-4 py-2 bg-gray-800 hover:bg-gray-700 text-gray-300 rounded-lg text-sm transition-colors"
          >
            &larr; Back
          </button>
          <button
            type="submit"
            disabled={isLoading || !idea.trim()}
            className="px-6 py-2 bg-blue-600 hover:bg-blue-500 text-white font-medium rounded-lg text-sm transition-colors disabled:opacity-50"
          >
            {isLoading ? (
              <span className="flex items-center gap-2">
                <span className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                Refining...
              </span>
            ) : (
              'Refine with AI'
            )}
          </button>
        </div>
      </form>
    </div>
  )
}
