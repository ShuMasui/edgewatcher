package com.edgewatcher.presentation;

import com.edgewatcher.domain.port.CredentialStore;
import dagger.MembersInjector;
import dagger.internal.DaggerGenerated;
import dagger.internal.InjectedFieldSignature;
import dagger.internal.Provider;
import dagger.internal.QualifierMetadata;
import javax.annotation.processing.Generated;

@QualifierMetadata
@DaggerGenerated
@Generated(
    value = "dagger.internal.codegen.ComponentProcessor",
    comments = "https://dagger.dev"
)
@SuppressWarnings({
    "unchecked",
    "rawtypes",
    "KotlinInternal",
    "KotlinInternalInJava",
    "cast",
    "deprecation",
    "nullness:initialization.field.uninitialized"
})
public final class MainActivity_MembersInjector implements MembersInjector<MainActivity> {
  private final Provider<CredentialStore> storeProvider;

  private MainActivity_MembersInjector(Provider<CredentialStore> storeProvider) {
    this.storeProvider = storeProvider;
  }

  public static MembersInjector<MainActivity> create(Provider<CredentialStore> storeProvider) {
    return new MainActivity_MembersInjector(storeProvider);
  }

  @Override
  public void injectMembers(MainActivity instance) {
    injectStore(instance, storeProvider.get());
  }

  @InjectedFieldSignature("com.edgewatcher.presentation.MainActivity.store")
  public static void injectStore(MainActivity instance, CredentialStore store) {
    instance.store = store;
  }
}
