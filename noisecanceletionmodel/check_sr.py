import torchaudio
import os

CLEAN_DIR = './data/speech-noise-kaggle/speech-noise-dataset/clean_speech'
NOISE_DIR = './data/speech-noise-kaggle/speech-noise-dataset/noise_only'
NOISY_DIR = './data/speech-noise-kaggle/speech-noise-dataset/noisy_speech'

def check_sr(directory):
    files = [f for f in os.listdir(directory) if f.endswith('.wav')]
    if files:
        first_file = os.path.join(directory, files[0])
        _, sr = torchaudio.load(first_file)
        print(f'SR in {directory}: {sr} Hz')
    else:
        print(f'No WAV files in {directory}')

check_sr(CLEAN_DIR)
check_sr(NOISY_DIR)
check_sr(NOISE_DIR)