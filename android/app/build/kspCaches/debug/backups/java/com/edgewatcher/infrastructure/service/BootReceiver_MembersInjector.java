package com.edgewatcher.infrastructure.service;

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
public final class BootReceiver_MembersInjector implements MembersInjector<BootReceiver> {
  private final Provider<CredentialStore> storeProvider;

  private BootReceiver_MembersInjector(Provider<CredentialStore> storeProvider) {
    this.storeProvider = storeProvider;
  }

  public static MembersInjector<BootReceiver> create(Provider<CredentialStore> storeProvider) {
    return new BootReceiver_MembersInjector(storeProvider);
  }

  @Override
  public void injectMembers(BootReceiver instance) {
    injectStore(instance, storeProvider.get());
  }

  @InjectedFieldSignature("com.edgewatcher.infrastructure.service.BootReceiver.store")
  public static void injectStore(BootReceiver instance, CredentialStore store) {
    instance.store = store;
  }
}
