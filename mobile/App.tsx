import { StatusBar } from 'expo-status-bar';
import { useState } from 'react';
import {
  AccessibilityInfo,
  Keyboard,
  Platform,
  Pressable,
  ScrollView,
  StatusBar as RNStatusBar,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';

// No routing backend exists yet, so a search only records the destination
// and says honestly that route search is not available.
type SearchState =
  | { kind: 'idle' }
  | { kind: 'empty' }
  | { kind: 'submitted'; destination: string };

const EMPTY_MESSAGE =
  'Tujuan belum diisi. Tulis nama tempat, halte, atau stasiun dulu, ya.';

export default function App() {
  const [destination, setDestination] = useState('');
  const [search, setSearch] = useState<SearchState>({ kind: 'idle' });

  const handleChange = (text: string) => {
    setDestination(text);
    if (search.kind !== 'idle') {
      setSearch({ kind: 'idle' });
    }
  };

  const handleSearch = () => {
    const trimmed = destination.trim();
    if (trimmed === '') {
      setSearch({ kind: 'empty' });
      AccessibilityInfo.announceForAccessibility(EMPTY_MESSAGE);
      return;
    }
    Keyboard.dismiss();
    setSearch({ kind: 'submitted', destination: trimmed });
    AccessibilityInfo.announceForAccessibility(
      `Tujuan ${trimmed} tercatat. Pencarian rute belum tersedia.`,
    );
  };

  const hasError = search.kind === 'empty';

  return (
    <View style={styles.root}>
      <StatusBar style="dark" />
      <ScrollView
        contentContainerStyle={styles.content}
        contentInsetAdjustmentBehavior="automatic"
        automaticallyAdjustKeyboardInsets
        keyboardShouldPersistTaps="handled"
      >
        <View style={styles.header}>
          <View style={styles.logo} accessible={false}>
            <View style={styles.logoDot} />
            <View style={styles.logoLine} />
            <View style={[styles.logoDot, styles.logoDotEnd]} />
          </View>
          <Text style={styles.title} accessibilityRole="header">
            Pandu Rute
          </Text>
          <Text style={styles.tagline}>Biar nggak bingung di jalan.</Text>
        </View>

        <View style={styles.card}>
          <Text style={styles.label} nativeID="destination-label">
            Tujuan
          </Text>
          <TextInput
            style={[styles.input, hasError && styles.inputError]}
            value={destination}
            onChangeText={handleChange}
            onSubmitEditing={handleSearch}
            placeholder="Mau ke mana?"
            placeholderTextColor={colors.placeholder}
            accessibilityLabel="Tujuan"
            accessibilityLabelledBy="destination-label"
            accessibilityHint="Tulis nama tempat, halte, atau stasiun tujuan"
            autoCapitalize="words"
            autoCorrect={false}
            returnKeyType="search"
            maxLength={100}
          />
          {hasError && (
            <Text
              style={styles.errorText}
              accessibilityLiveRegion="polite"
            >
              {EMPTY_MESSAGE}
            </Text>
          )}

          <Pressable
            onPress={handleSearch}
            accessibilityRole="button"
            accessibilityHint="Mencari rute ke tujuan yang ditulis"
            style={({ pressed }) => [styles.button, pressed && styles.buttonPressed]}
          >
            <Text style={styles.buttonText}>Cari Rute</Text>
          </Pressable>
        </View>

        {search.kind === 'submitted' && (
          <View style={styles.notice} accessibilityLiveRegion="polite">
            <Text style={styles.noticeTitle}>Tujuan tercatat</Text>
            <Text style={styles.noticeDestination}>“{search.destination}”</Text>
            <Text style={styles.noticeBody}>
              Pencarian rute belum tersedia. Fitur ini masih dibangun, jadi
              belum ada rute yang bisa ditampilkan.
            </Text>
          </View>
        )}

        <Text style={styles.footer}>
          Versi awal untuk transportasi umum Jakarta. Data rute masih dalam
          pengembangan.
        </Text>
      </ScrollView>
    </View>
  );
}

const colors = {
  background: '#F4F7FA',
  surface: '#FFFFFF',
  primary: '#0E5A8A',
  primaryPressed: '#0A4469',
  text: '#13212E',
  muted: '#4B5B69',
  border: '#7D8D9C',
  placeholder: '#5F6E7C',
  error: '#B42318',
  errorBackground: '#FEF3F2',
  noticeBackground: '#E8F1F8',
};

const styles = StyleSheet.create({
  root: {
    flex: 1,
    backgroundColor: colors.background,
  },
  content: {
    flexGrow: 1,
    paddingHorizontal: 24,
    paddingTop: (Platform.OS === 'android' ? RNStatusBar.currentHeight ?? 0 : 0) + 32,
    paddingBottom: 32,
  },
  header: {
    marginBottom: 28,
  },
  logo: {
    flexDirection: 'row',
    alignItems: 'center',
    marginBottom: 20,
  },
  logoDot: {
    width: 14,
    height: 14,
    borderRadius: 7,
    borderWidth: 3,
    borderColor: colors.primary,
    backgroundColor: colors.surface,
  },
  logoDotEnd: {
    backgroundColor: colors.primary,
  },
  logoLine: {
    width: 36,
    height: 3,
    backgroundColor: colors.primary,
  },
  title: {
    fontSize: 34,
    fontWeight: '800',
    color: colors.text,
    letterSpacing: -0.5,
  },
  tagline: {
    marginTop: 6,
    fontSize: 17,
    lineHeight: 24,
    color: colors.muted,
  },
  card: {
    backgroundColor: colors.surface,
    borderRadius: 16,
    padding: 20,
    borderWidth: 1,
    borderColor: '#E1E7ED',
  },
  label: {
    fontSize: 15,
    fontWeight: '600',
    color: colors.text,
    marginBottom: 8,
  },
  input: {
    minHeight: 52,
    borderWidth: 1.5,
    borderColor: colors.border,
    borderRadius: 12,
    paddingHorizontal: 16,
    fontSize: 17,
    color: colors.text,
    backgroundColor: colors.surface,
  },
  inputError: {
    borderColor: colors.error,
    backgroundColor: colors.errorBackground,
  },
  errorText: {
    marginTop: 8,
    fontSize: 15,
    lineHeight: 21,
    color: colors.error,
  },
  button: {
    marginTop: 16,
    minHeight: 52,
    borderRadius: 12,
    backgroundColor: colors.primary,
    alignItems: 'center',
    justifyContent: 'center',
    paddingHorizontal: 16,
  },
  buttonPressed: {
    backgroundColor: colors.primaryPressed,
  },
  buttonText: {
    fontSize: 17,
    fontWeight: '700',
    color: '#FFFFFF',
  },
  notice: {
    marginTop: 20,
    padding: 16,
    borderRadius: 12,
    backgroundColor: colors.noticeBackground,
    borderLeftWidth: 4,
    borderLeftColor: colors.primary,
  },
  noticeTitle: {
    fontSize: 15,
    fontWeight: '700',
    color: colors.primary,
  },
  noticeDestination: {
    marginTop: 4,
    fontSize: 17,
    fontWeight: '600',
    color: colors.text,
  },
  noticeBody: {
    marginTop: 8,
    fontSize: 15,
    lineHeight: 22,
    color: colors.muted,
  },
  footer: {
    marginTop: 'auto',
    paddingTop: 32,
    fontSize: 13,
    lineHeight: 19,
    color: colors.muted,
    textAlign: 'center',
  },
});
